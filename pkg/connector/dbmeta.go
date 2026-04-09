package connector

import (
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
)

func (bc *BoostyConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Reaction: nil,
		Portal:   nil,
		Message:  nil,
		Ghost:    nil,
		UserLogin: func() any {
			return &UserLoginMetadata{}
		},
	}
}

// UserLoginMetadata stores Boosty authentication data in the bridge database.
type UserLoginMetadata struct {
	boosty.AuthData
}
