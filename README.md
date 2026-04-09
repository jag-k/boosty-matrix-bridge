# mautrix-boosty

A Matrix-Boosty DM puppeting bridge based on [mautrix-go bridgev2](https://github.com/mautrix/go).

## Features

- Bridge Boosty direct messages to Matrix
- Email/password login
- Text messages (send and receive)
- Read receipts
- Message history backfill
- Polling-based message sync

## Building

```bash
# With goolm (pure Go, no libolm needed)
go build -tags goolm ./cmd/mautrix-boosty/

# With libolm (if installed via homebrew on macOS)
CGO_CFLAGS="-I/opt/homebrew/include" CGO_LDFLAGS="-L/opt/homebrew/lib" go build ./cmd/mautrix-boosty/
```

## Docker

```bash
docker build -t mautrix-boosty .
docker run -v /path/to/data:/data mautrix-boosty
```

## Configuration

1. Generate an example config:

   ```bash
   ./mautrix-boosty -c config.yaml -e
   ```

2. Edit `config.yaml` with your homeserver details.
3. Generate the appservice registration file:

   ```bash
   ./mautrix-boosty -c config.yaml -r registration.yaml -g
   ```

4. Start the bridge:

   ```bash
   ./mautrix-boosty -c config.yaml
   ```

## Connecting to Beeper

1. Generate a config for the bridge (no need to run `bbctl registry`):

   ```bash
   bbctl config --type bridgev2 -o config.yaml sh-boosty
   ```

2. Start the bridge with the generated config:

   ```bash
   ./mautrix-boosty -c config.yaml
   ```

## Login

After the bridge is running, start a chat with the bridge bot and use the login command. You will be prompted for your Boosty email and password.

## Network Configuration

Bridge-specific settings in `config.yaml` under the `network` key:

```yaml
network:
  displayname_template: "{{.Name}}"
  poll_interval: 30
```

- `displayname_template` - Go template for ghost display names
- `poll_interval` - How often to poll Boosty for new messages (seconds)
