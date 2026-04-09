package connector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"

	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
)

type BoostyClient struct {
	main      *BoostyConnector
	userID    networkid.UserID
	userLogin *bridgev2.UserLogin
	client    *boosty.Client

	stopPolling context.CancelFunc
	lastSeenMsg map[string]string // dialog ID -> last seen message ID
	lastReadMsg map[string]string // dialog ID -> last read message ID (sent by us, read by chatmate)
}

var _ bridgev2.NetworkAPI = (*BoostyClient)(nil)

func NewBoostyClient(ctx context.Context, bc *BoostyConnector, login *bridgev2.UserLogin) *BoostyClient {
	meta := login.Metadata.(*UserLoginMetadata)
	client := boosty.NewClient(meta.AuthData)
	client.OnTokenRefresh = func(ctx context.Context, auth boosty.AuthData) {
		meta.AuthData = auth
		if err := login.Save(ctx); err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save refreshed tokens")
		}
	}

	return &BoostyClient{
		main:        bc,
		userID:      networkid.UserID(login.ID),
		userLogin:   login,
		client:      client,
		lastSeenMsg: make(map[string]string),
		lastReadMsg: make(map[string]string),
	}
}

func (b *BoostyClient) Connect(ctx context.Context) {
	if !b.client.IsLoggedIn() {
		b.userLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "boosty-no-auth",
			Message:    "Not logged in to Boosty",
		})
		return
	}

	b.userLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})

	// Validate credentials by fetching current user
	_, err := b.client.GetCurrentUser(ctx)
	if err != nil {
		b.userLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "boosty-auth-failed",
			Message:    fmt.Sprintf("Failed to validate credentials: %v", err),
		})
		return
	}

	b.userLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})

	log := b.userLogin.Log.With().Str("component", "poller").Logger()
	pollCtx, cancel := context.WithCancel(log.WithContext(context.Background()))
	b.stopPolling = cancel
	go b.pollLoop(pollCtx)
}

func (b *BoostyClient) Disconnect() {
	if b.stopPolling != nil {
		b.stopPolling()
		b.stopPolling = nil
	}
}

func (b *BoostyClient) IsLoggedIn() bool {
	return b.client.IsLoggedIn()
}

func (b *BoostyClient) LogoutRemote(ctx context.Context) {
	b.Disconnect()
}

func (b *BoostyClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return b.userID == userID
}

func (b *BoostyClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	dialog, err := b.client.GetDialog(ctx, string(portal.ID))
	if err != nil {
		return nil, err
	}
	return b.dialogToChatInfo(dialog), nil
}

func (b *BoostyClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return nil, nil
}

func (b *BoostyClient) HandleMatrixReadReceipt(ctx context.Context, msg *bridgev2.MatrixReadReceipt) error {
	if !b.IsLoggedIn() {
		return bridgev2.ErrNotLoggedIn
	}
	if msg.ExactMessage != nil {
		return b.client.MarkMessageRead(ctx, string(msg.ExactMessage.ID))
	}
	return nil
}

func (b *BoostyClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if !b.IsLoggedIn() {
		return nil, bridgev2.ErrNotLoggedIn
	}

	text := msg.Content.Body
	dialogID := string(msg.Portal.ID)
	sentMsg, err := b.client.SendMessage(ctx, dialogID, text)
	if err != nil {
		return nil, err
	}

	b.lastSeenMsg[dialogID] = strconv.Itoa(sentMsg.ID)

	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        networkid.MessageID(strconv.Itoa(sentMsg.ID)),
			MXID:      msg.Event.ID,
			Room:      msg.Portal.PortalKey,
			SenderID:  b.userID,
			Timestamp: sentMsg.CreatedAtTime(),
		},
	}, nil
}

func (b *BoostyClient) pollLoop(ctx context.Context) {
	log := b.userLogin.Log.With().Str("action", "poll").Logger()
	interval := time.Duration(b.main.Config.PollInterval) * time.Second
	if interval < 10*time.Second {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial sync
	b.syncDialogs(log.WithContext(ctx))

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Polling stopped")
			return
		case <-ticker.C:
			b.syncDialogs(log.WithContext(ctx))
		}
	}
}

func (b *BoostyClient) syncDialogs(ctx context.Context) {
	log := zerolog.Ctx(ctx)

	dialogs, err := b.client.GetDialogs(ctx, 50)
	if err != nil {
		log.Err(err).Msg("Failed to fetch dialogs")
		return
	}

	for _, dialog := range dialogs.Data {
		dialogIDStr := strconv.Itoa(dialog.ID)
		portalKey := networkid.PortalKey{
			ID:       networkid.PortalID(dialogIDStr),
			Receiver: b.userLogin.ID,
		}

		if dialog.LastMessage == nil {
			continue
		}

		msg := dialog.LastMessage
		msgIDStr := strconv.Itoa(msg.ID)

		lastSeen, seenBefore := b.lastSeenMsg[dialogIDStr]
		if !seenBefore {
			chatInfo := b.dialogToChatInfoFromList(&dialog)
			b.main.Bridge.QueueRemoteEvent(b.userLogin, &simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type: bridgev2.RemoteEventChatResync,
					LogContext: func(c zerolog.Context) zerolog.Context {
						return c.Str("dialog_id", dialogIDStr)
					},
					PortalKey:    portalKey,
					CreatePortal: true,
				},
				ChatInfo:        chatInfo,
				LatestMessageTS: msg.CreatedAtTime(),
			})
			b.lastSeenMsg[dialogIDStr] = msgIDStr
			continue
		}

		b.checkAndEmitReadReceipt(dialogIDStr, &dialog, msg, portalKey)

		if lastSeen == msgIDStr {
			continue
		}

		b.fetchAndEmitNewMessages(ctx, dialogIDStr, lastSeen, portalKey)
	}
}

// checkAndEmitReadReceipt emits a read receipt from the chatmate when a message
// sent by the logged-in user transitions to isRead=true.
func (b *BoostyClient) checkAndEmitReadReceipt(dialogID string, dialog *boosty.Dialog, msg *boosty.Message, portalKey networkid.PortalKey) {
	isFromMe := msg.AuthorID != dialog.Chatmate.ID
	if !isFromMe || !msg.IsRead {
		return
	}

	msgIDStr := strconv.Itoa(msg.ID)
	lastRead := b.lastReadMsg[dialogID]
	if lastRead == msgIDStr {
		return
	}
	b.lastReadMsg[dialogID] = msgIDStr

	chatmateID := networkid.UserID(strconv.Itoa(dialog.Chatmate.ID))
	b.main.Bridge.QueueRemoteEvent(b.userLogin, &simplevent.Receipt{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventReadReceipt,
			PortalKey: portalKey,
			Sender: bridgev2.EventSender{
				Sender: chatmateID,
			},
		},
		LastTarget: networkid.MessageID(msgIDStr),
	})
}

func (b *BoostyClient) fetchAndEmitNewMessages(ctx context.Context, dialogID, lastSeenID string, portalKey networkid.PortalKey) {
	log := zerolog.Ctx(ctx)

	detail, err := b.client.GetDialog(ctx, dialogID)
	if err != nil {
		log.Err(err).Str("dialog_id", dialogID).Msg("Failed to fetch dialog for new messages")
		return
	}

	// Boosty API returns messages oldest-first.
	// Collect messages that appear AFTER lastSeenID in the list.
	var newMessages []boosty.Message
	found := false
	for _, m := range detail.Messages.Data {
		if strconv.Itoa(m.ID) == lastSeenID {
			found = true
			continue
		}
		if found {
			newMessages = append(newMessages, m)
		}
	}

	if len(newMessages) == 0 {
		if found {
			return
		}
		msgs := detail.Messages.Data
		if len(msgs) > 0 {
			b.lastSeenMsg[dialogID] = strconv.Itoa(msgs[len(msgs)-1].ID)
		}
		return
	}

	b.lastSeenMsg[dialogID] = strconv.Itoa(newMessages[len(newMessages)-1].ID)

	for _, m := range newMessages {
		mIDStr := strconv.Itoa(m.ID)

		senderID := networkid.UserID(strconv.Itoa(detail.Chatmate.ID))
		isFromMe := m.AuthorID != detail.Chatmate.ID
		if isFromMe {
			senderID = b.userID
		}

		sender := bridgev2.EventSender{
			IsFromMe: isFromMe,
			Sender:   senderID,
		}
		if isFromMe {
			sender.SenderLogin = b.userLogin.ID
		}

		b.main.Bridge.QueueRemoteEvent(b.userLogin, &simplevent.Message[boosty.Message]{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventMessage,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.
						Str("message_id", mIDStr).
						Str("dialog_id", dialogID)
				},
				PortalKey:    portalKey,
				Sender:       sender,
				Timestamp:    m.CreatedAtTime(),
				CreatePortal: true,
			},
			ID:                 networkid.MessageID(mIDStr),
			Data:               m,
			ConvertMessageFunc: b.convertToMatrix,
		})
	}

	var latestReadByMe *boosty.Message
	for i := len(detail.Messages.Data) - 1; i >= 0; i-- {
		m := detail.Messages.Data[i]
		isFromMe := m.AuthorID != detail.Chatmate.ID
		if isFromMe && m.IsRead {
			latestReadByMe = &m
			break
		}
	}
	if latestReadByMe != nil {
		readIDStr := strconv.Itoa(latestReadByMe.ID)
		lastRead := b.lastReadMsg[dialogID]
		if lastRead != readIDStr {
			b.lastReadMsg[dialogID] = readIDStr
			chatmateID := networkid.UserID(strconv.Itoa(detail.Chatmate.ID))
			b.main.Bridge.QueueRemoteEvent(b.userLogin, &simplevent.Receipt{
				EventMeta: simplevent.EventMeta{
					Type:      bridgev2.RemoteEventReadReceipt,
					PortalKey: portalKey,
					Sender: bridgev2.EventSender{
						Sender: chatmateID,
					},
				},
				LastTarget: networkid.MessageID(readIDStr),
			})
		}
	}

	log.Info().
		Str("dialog_id", dialogID).
		Int("count", len(newMessages)).
		Msg("Emitted new messages")
}

func (b *BoostyClient) dialogToChatInfo(detail *boosty.DialogDetailResponse) *bridgev2.ChatInfo {
	chatmateID := networkid.UserID(strconv.Itoa(detail.Chatmate.ID))
	name := b.main.Config.FormatDisplayname(DisplaynameParams{Name: detail.Chatmate.Name})

	return &bridgev2.ChatInfo{
		Name:        &name,
		Avatar:      b.wrapAvatar(detail.Chatmate.AvatarURL),
		Type:        ptr.Ptr(database.RoomTypeDM),
		CanBackfill: true,
		Members: &bridgev2.ChatMemberList{
			IsFull:           false,
			TotalMemberCount: 2,
			OtherUserID:      chatmateID,
			MemberMap: map[networkid.UserID]bridgev2.ChatMember{
				chatmateID: {
					EventSender: bridgev2.EventSender{
						Sender: chatmateID,
					},
					Membership: event.MembershipJoin,
					UserInfo: &bridgev2.UserInfo{
						Name:        &name,
						Avatar:      b.wrapAvatar(detail.Chatmate.AvatarURL),
						Identifiers: []string{fmt.Sprintf("boosty:%d", detail.Chatmate.ID)},
					},
				},
			},
		},
	}
}

func (b *BoostyClient) dialogToChatInfoFromList(dialog *boosty.Dialog) *bridgev2.ChatInfo {
	chatmateID := networkid.UserID(strconv.Itoa(dialog.Chatmate.ID))
	name := b.main.Config.FormatDisplayname(DisplaynameParams{Name: dialog.Chatmate.Name})

	return &bridgev2.ChatInfo{
		Name:        &name,
		Avatar:      b.wrapAvatar(dialog.Chatmate.AvatarURL),
		Type:        ptr.Ptr(database.RoomTypeDM),
		CanBackfill: true,
		Members: &bridgev2.ChatMemberList{
			IsFull:           false,
			TotalMemberCount: 2,
			OtherUserID:      chatmateID,
			MemberMap: map[networkid.UserID]bridgev2.ChatMember{
				chatmateID: {
					EventSender: bridgev2.EventSender{
						Sender: chatmateID,
					},
					Membership: event.MembershipJoin,
					UserInfo: &bridgev2.UserInfo{
						Name:        &name,
						Avatar:      b.wrapAvatar(dialog.Chatmate.AvatarURL),
						Identifiers: []string{fmt.Sprintf("boosty:%d", dialog.Chatmate.ID)},
					},
				},
			},
		},
	}
}

func (b *BoostyClient) wrapAvatar(avatarURL string) *bridgev2.Avatar {
	if avatarURL == "" {
		return &bridgev2.Avatar{Remove: true}
	}
	return &bridgev2.Avatar{
		ID: networkid.AvatarID(avatarURL),
		Get: func(ctx context.Context) ([]byte, error) {
			return b.client.DownloadAvatar(ctx, avatarURL)
		},
	}
}
