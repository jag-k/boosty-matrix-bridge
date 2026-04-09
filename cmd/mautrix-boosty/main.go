package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"github.com/jag-k/boosty-matrix-bridge/pkg/connector"
)

// Information to find out exactly which commit the bridge was built from.
// These are filled at build time with the -X linker flag.
var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	m := mxmain.BridgeMain{
		Name:        "mautrix-boosty",
		URL:         "https://github.com/jag-k/boosty-matrix-bridge",
		Description: "A Matrix-Boosty DM puppeting bridge.",
		Version:     Tag,
		Connector:   &connector.BoostyConnector{},
	}
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
