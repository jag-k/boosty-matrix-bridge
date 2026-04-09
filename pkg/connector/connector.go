package connector

import (
	"context"
	_ "embed"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

//go:embed icon.png
var boostyIcon []byte

type BoostyConnector struct {
	Bridge      *bridgev2.Bridge
	Config      Config
	networkIcon id.ContentURIString
}

var _ bridgev2.NetworkConnector = (*BoostyConnector)(nil)

func (bc *BoostyConnector) Init(bridge *bridgev2.Bridge) {
	bc.Bridge = bridge
}

func (bc *BoostyConnector) Start(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	iconURI, _, err := bc.Bridge.Bot.UploadMedia(ctx, "", boostyIcon, "icon.png", "image/png")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to upload network icon, bridge will work without it")
	} else {
		bc.networkIcon = iconURI
	}
	return nil
}

func (bc *BoostyConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:      "Boosty",
		NetworkURL:       "https://boosty.to",
		NetworkIcon:      bc.networkIcon,
		NetworkID:        "boosty",
		BeeperBridgeType: "sh-boosty",
		DefaultPort:      29380,
	}
}

func (bc *BoostyConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	login.Client = NewBoostyClient(ctx, bc, login)
	return nil
}
