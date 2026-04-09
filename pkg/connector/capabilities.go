package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

func (*BoostyConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return &bridgev2.NetworkGeneralCapabilities{}
}

func (*BoostyConnector) GetBridgeInfoVersion() (info, capabilities int) {
	return 1, 1
}

const MaxTextLength = 4000

func (*BoostyClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	return &event.RoomFeatures{
		ID:            "fi.mau.boosty.capabilities.2026_04",
		Formatting:    event.FormattingFeatureMap{},
		MaxTextLength: MaxTextLength,
		ReadReceipts:  true,
		Reaction:      event.CapLevelRejected,
		Edit:          event.CapLevelRejected,
		Reply:         event.CapLevelRejected,
	}
}
