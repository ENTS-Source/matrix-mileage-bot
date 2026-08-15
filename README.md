# Matrix Mileage Bot

A small Matrix bot for recording mileage reimbursements and generating one PDF reimbursement report per person.

It uses:

- [`maunium.net/go/mautrix`](https://pkg.go.dev/maunium.net/go/mautrix) for Matrix client/sync/media handling.
- BoltDB (`bbolt`) for local persistent records and Matrix sync state.
- `go-pdf/fpdf` for PDF generation.

## Commands

| Command | Who can use it | Behavior |
|---|---|---|
| `!mileage start <odometer>` | Anyone | Starts one active odometer record for that Matrix user. |
| `!mileage end <odometer>` | Anyone | Ends that user's active record and records the odometer difference. |
| `!mileage <km>` | Anyone | Records a standalone trip. The start and end date are both the command date. |
| `!mileage report` | Configured authorized users | Generates and uploads one PDF for every user with completed records. |
| `!mileage reset` | Configured authorized users | Deletes all completed records and active odometer records. Matrix sync state is retained. |

The bot stores dates in the configured timezone (default `America/Edmonton`).

## Reimbursement calculation

Rates are progressive and calculated separately for each person's total completed mileage since the last reset. With the example configuration:

- first 5,000 km: $0.73/km
- mileage above 5,000 km: $0.67/km

For example, 6,000 km is `5000 × $0.73 + 1000 × $0.67 = $4,320.00`.

Each PDF contains the user's current Matrix display name (when available), full Matrix user ID, the record table, rate breakdown, total mileage, and total reimbursement.

Filenames are:

```text
123.45 - username - mileage.pdf
```

where `username` is the Matrix localpart and `123.45` is the reimbursement total.

## Configuration

Copy `config.example.yaml` to `config.yaml` and edit it:

```yaml
matrix:
  homeserver: "https://matrix.example.org"
  user_id: "@mileage:example.org"
  access_token: "YOUR_ACCESS_TOKEN"
  allowed_room_ids:
    - "!yourRoomId:example.org"

authorized_users:
  - "@treasurer:example.org"

purpose: "2026 Move"
timezone: "America/Edmonton"
storage_path: "./mileage.db"

reimbursement:
  currency: "CAD"
  tiers:
    - up_to_km: 5000
      rate_per_km: 0.73
    - rate_per_km: 0.67
```

`allowed_room_ids` is optional. If it is empty or omitted, commands are accepted in any joined room.

The final reimbursement tier must omit `up_to_km`; this makes it the catch-all rate for mileage above the previous tier.

## Getting an access token

Create a dedicated Matrix account for the bot and obtain an access token for that account. Put the token in `config.yaml` and keep the file private. Invite the account to the room(s) where it should operate.

## Run locally

Requires Go 1.25+ because mautrix-go v0.29.0 requires Go 1.25.

```sh
go mod tidy
go test ./...
go run . -config config.yaml
```

On first startup, the bot deliberately ignores existing room history and begins responding to new commands. Its Matrix `/sync` token is then persisted in `mileage.db`, preventing old commands from being replayed after restarts.

## Docker

```sh
docker build -t matrix-mileage-bot .
docker run --rm \
  -v "$PWD/config.yaml:/data/config.yaml:ro" \
  -v "$PWD/data:/data/storage" \
  matrix-mileage-bot -config /data/config.yaml
```

When using that volume layout, set:

```yaml
storage_path: "/data/storage/mileage.db"
```

## Encryption

This intentionally minimal build handles **unencrypted Matrix rooms**. Mautrix-go has end-to-end encryption support, but enabling it requires persistent crypto/device state and adds substantial operational complexity. If the room is encrypted, either create a dedicated unencrypted room for mileage submissions or extend the client with mautrix-go's crypto helper.

## Operational notes

- A user can have at most one active odometer record.
- Ending odometer must be greater than starting odometer.
- Distances support up to three decimal places (meter precision).
- `report` and `reset` are both restricted to `authorized_users` because reset is destructive.
- `reset` intentionally leaves Matrix sync state intact so old commands do not become eligible for replay.
- The configured purpose is copied into each record at creation time.
