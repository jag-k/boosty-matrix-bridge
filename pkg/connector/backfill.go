package connector

import (
	"context"
	"strconv"

	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

var _ bridgev2.BackfillingNetworkAPI = (*BoostyClient)(nil)

func (b *BoostyClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	log := zerolog.Ctx(ctx).With().Str("method", "fetch_messages").Logger()
	log.Info().Str("portal_id", string(params.Portal.ID)).Msg("FetchMessages called")
	ctx = log.WithContext(ctx)

	portal, err := b.main.Bridge.GetPortalByKey(ctx, params.Portal.PortalKey)
	if err != nil {
		return nil, err
	}

	anchor := ""
	if params.AnchorMessage != nil {
		anchor = string(params.AnchorMessage.ID)
	}
	if !params.Forward && params.Cursor != "" {
		anchor = string(params.Cursor)
	}
	detail, err := b.client.GetMessageHistory(ctx, string(params.Portal.ID), anchor, params.Forward, params.Count)
	if err != nil {
		return nil, err
	}

	if len(detail.Messages.Data) == 0 {
		return &bridgev2.FetchMessagesResponse{HasMore: false, Forward: params.Forward}, nil
	}

	resp := bridgev2.FetchMessagesResponse{
		Forward: params.Forward,
		HasMore: !detail.Messages.Extra.IsFirst,
		Cursor:  networkid.PaginationCursor(strconv.Itoa(detail.Messages.Data[0].ID)),
	}

	chatmateID := networkid.UserID(strconv.Itoa(detail.Chatmate.ID))

	for _, msg := range messagesInBackfillWindow(detail.Messages.Data, params.AnchorMessage, params.Forward) {
		senderID := chatmateID
		isFromMe := msg.AuthorID != detail.Chatmate.ID
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

		intent, ok := portal.GetIntentFor(ctx, sender, b.userLogin, bridgev2.RemoteEventBackfill)
		if !ok {
			continue
		}

		converted, err := b.convertToMatrix(ctx, portal, intent, msg)
		if err != nil {
			log.Err(err).Int("message_id", msg.ID).Msg("Failed to convert message for backfill")
			continue
		}

		resp.Messages = append(resp.Messages, &bridgev2.BackfillMessage{
			ConvertedMessage: converted,
			Sender:           sender,
			ID:               networkid.MessageID(strconv.Itoa(msg.ID)),
			Timestamp:        msg.CreatedAtTime(),
		})
	}

	log.Info().Int("count", len(resp.Messages)).Msg("Backfilled messages")
	return &resp, nil
}

var _ bridgev2.BackfillingNetworkAPIWithLimits = (*BoostyClient)(nil)

func (b *BoostyClient) GetBackfillMaxBatchCount(ctx context.Context, portal *bridgev2.Portal, task *database.BackfillTask) int {
	// Boosty currently supports only one-to-one dialogs.
	return b.main.Bridge.Config.Backfill.Queue.GetOverride("dm")
}

func messagesInBackfillWindow(messages []boosty.Message, anchor *database.Message, forward bool) []boosty.Message {
	if anchor == nil {
		return messages
	}
	for i, msg := range messages {
		if strconv.Itoa(msg.ID) == string(anchor.ID) {
			if forward {
				return messages[i+1:]
			}
			return messages[:i]
		}
	}
	// The anchor may no longer occur in the API's limited page. Do not re-send
	// the whole page, or drop all newer messages just because it is absent.
	var result []boosty.Message
	anchorID, _ := strconv.Atoi(string(anchor.ID))
	for _, msg := range messages {
		ts := msg.CreatedAtTime()
		newer := ts.After(anchor.Timestamp) || (ts.Equal(anchor.Timestamp) && msg.ID > anchorID)
		older := ts.Before(anchor.Timestamp) || (ts.Equal(anchor.Timestamp) && msg.ID < anchorID)
		if (forward && newer) || (!forward && older) {
			result = append(result, msg)
		}
	}
	return result
}
