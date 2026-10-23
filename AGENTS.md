# AGENTS.md

## Project overview

**mautrix-boosty** is a Matrix bridge for Boosty direct messages, built with Go and [mautrix-go bridgev2](https://github.com/mautrix/go/tree/master/bridgev2). Runs as a self-hosted Beeper bridge (`sh-boosty`); `bbctl` is only used to generate the initial config.

## Tech stack

- **Language:** Go 1.26+
- **Framework:** mautrix-go bridgev2 (handles E2BE, double puppeting, backfill, provisioning)
- **Database:** SQLite (via mautrix bridgev2)
- **Logging:** zerolog
- **Crypto:** goolm (pure Go, no libolm/CGO needed for olm — but CGO is required for sqlite3)
- **Container:** Docker multi-stage (golang:alpine → alpine), published to GHCR

## Code structure

- `cmd/mautrix-boosty/` — entry point
- `pkg/boosty/` — standalone Boosty HTTP API client (auth, dialogs, messages, content parsing)
- `pkg/connector/` — bridgev2 connector (polling, login flows, message conversion, media, capabilities)
- `.github/workflows/build.yml` — CI with matrix strategy for multi-arch Docker builds

## Key conventions

- All Boosty IDs (`Dialog.ID`, `Message.ID`, `DialogID`) are `int`, converted to string via `strconv.Itoa()` when used as `networkid.`*
- Boosty API returns messages in **oldest-first** order
- Login is cookie-based only (browser extraction or manual JSON paste), no email/password flow
- Polling-based sync (no WebSocket/Centrifugo yet)
- HTML output uses `html.EscapeString()` for all user-provided content
- Reactions are explicitly disabled (`CapLevelRejected`)

## Building

```bash
# Full binary (requires CGO for sqlite3)
go build -tags goolm ./cmd/mautrix-boosty/

# Docker
docker build -t mautrix-boosty .
```

## Running with Beeper

```bash
bbctl config --type bridgev2 -o config.yaml sh-boosty
./mautrix-boosty -c config.yaml
```

## Detailed plan

See [.ai/plans/boosty-bridge.md](.ai/plans/boosty-bridge.md) for full architecture, API docs, and implementation status.