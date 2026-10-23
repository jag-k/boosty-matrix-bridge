# mautrix-boosty

A Matrix-Boosty DM puppeting bridge based on [mautrix-go bridgev2](https://github.com/mautrix/go).

## Features

- Bridge Boosty direct messages to Matrix
- Browser cookie extraction or manual auth cookie JSON login
- Text messages (send and receive)
- Receive images, files, audio and stickers; video links
- Read receipts
- Paginated message history and catch-up
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

After the bridge is running, start a chat with the bridge bot and use the login command. Choose browser cookie extraction or paste the `auth` cookie JSON from Boosty DevTools. Percent-encoded cookie values are accepted. The `_clientId` cookie can be supplied as the device ID. Access tokens refresh automatically on HTTP 401, and refreshed credentials are saved to the login database. If the refresh token is rejected or revoked, log in again.

## Network Configuration

Bridge-specific settings in `config.yaml` under the `network` key:

```yaml
network:
  displayname_template: "{{.Name}}"
  poll_interval: 30
```

- `displayname_template` - Go template for ghost display names
- `poll_interval` - How often to poll Boosty for new messages (seconds)


## History and attachments

Enable `backfill.enabled` and `backfill.queue.enabled` in `config.yaml`.
The connector uses `backfill.queue.max_batches_override.dm` for DM history,
falling back to `backfill.queue.max_batches`. Zero disables queued history;
`-1` allows all batches the connector can fetch. For example:

```yaml
backfill:
  enabled: true
  max_initial_messages: 50
  max_catchup_messages: 500
  queue:
    enabled: true
    batch_size: 50
    batch_delay: 20
    max_batches: 0
    max_batches_override:
      dm: -1
```

The bridge fetches older messages through Boosty's directional message API,
using message IDs as cursors. Initial sync collects recent history up to
`max_initial_messages`, the queue continues backwards, and catch-up drains all
newer pages after the last known message. Pages are deduplicated and sorted
oldest first. A stalled cursor is treated as an error rather than silently
marking history complete.

Incoming files and audio use the dialog's `signedQuery`; image URLs already carry
their own signature. Failed attachment downloads produce a visible notice.
Matrix-to-Boosty sending currently supports text only. Native Boosty video is
represented by a stream link rather than an uploaded video file.

If Boosty rejects the saved refresh token (`invalid_grant`), a fresh login is
required. The bridge cannot obtain new credentials automatically after a revoked
or otherwise invalid refresh token. Use the browser login flow or paste a fresh
`auth` cookie along with the matching `_clientId`; do not paste credentials into
logs or issue reports.

If an older version logged `Event #... has same ID` followed by database insert
errors, that Matrix batch was rejected. The framework may still have stored
message mappings locally, so restarting the fixed bridge alone may not recover
that batch. Preserve the database and logs before recovery; removing the entire
database also removes login and encryption state. Repair must reconcile the
failed batch's stored mappings with the events actually present on the server.
