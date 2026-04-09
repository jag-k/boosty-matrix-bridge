package connector

import (
	"context"
	"strconv"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
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

	// Boosty dialog API returns messages in the dialog detail response.
	// We don't have cursor-based pagination yet, so we only support initial backfill.
	if params.Cursor != "" {
		return &bridgev2.FetchMessagesResponse{HasMore: false, Forward: params.Forward}, nil
	}

	detail, err := b.client.GetDialog(ctx, string(params.Portal.ID))
	if err != nil {
		return nil, err
	}

	if len(detail.Messages.Data) == 0 {
		return &bridgev2.FetchMessagesResponse{HasMore: false, Forward: params.Forward}, nil
	}

	resp := bridgev2.FetchMessagesResponse{
		Forward: params.Forward,
		HasMore: false, // no pagination support yet
	}

	chatmateID := networkid.UserID(strconv.Itoa(detail.Chatmate.ID))

	for _, msg := range detail.Messages.Data {
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
