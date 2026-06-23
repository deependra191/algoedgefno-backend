# Kite Backfill

Local-only Zerodha Kite intraday importer for strategy validation.

By default this command imports Kite `minute` candles as repo interval `1m` with
provider `zerodha_kite`. It also has an explicit `-daily` fallback mode that
imports Kite `day` candles as repo interval `1d` for older daily history where
the NSE bhavcopy parser does not cover the source format.

## Clean Session Setup

Start local TimescaleDB:

```bash
docker compose -f docker/docker-compose.yml up -d
```

Export local backend database config in the shell that will run the importer:

```bash
export APP_ENV=development
export JWT_SECRET='local-dev-only-random-value'
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=algoedge
export DB_PASSWORD=algoedge
export DB_NAME=algoedgefno
export DB_SSL_REQUIRED=false
export AUTO_MIGRATE=false
```

Apply migrations before running the importer. The importer deliberately disables
auto-migrations on its own path.

```bash
migrate -path migrations/ \
  -database 'postgres://algoedge:algoedge@localhost:5432/algoedgefno?sslmode=disable' \
  up
```

If the `migrate` CLI is not installed, run the development server once with
auto-migration enabled, then stop it before running the importer:

```bash
AUTO_MIGRATE=true go run ./cmd/server
```

## Kite Token Setup

All commands in this section must be run from the repo root (the directory
containing `go.mod`). If you are using a git worktree, `cd` into it first.

Export the Kite app credentials locally:

```bash
export KITE_API_KEY='your_api_key'
export KITE_API_SECRET='your_api_secret'
```

Open the login URL:

```bash
printf 'https://kite.zerodha.com/connect/login?v=3&api_key=%s\n' "$KITE_API_KEY"
```

After login, Zerodha redirects to your configured redirect URL with a
`request_token` query parameter. Exchange only that request token.

On **zsh** (macOS default):

```zsh
read -rs "REQUEST_TOKEN?Kite request_token: "; echo
go run ./local-rnd/kite-token \
  -request-token "$REQUEST_TOKEN" \
  -out /private/tmp/kite-access-token.env
unset REQUEST_TOKEN
source /private/tmp/kite-access-token.env
```

On **bash**:

```bash
read -rsp 'Kite request_token: ' REQUEST_TOKEN; echo
go run ./local-rnd/kite-token \
  -request-token "$REQUEST_TOKEN" \
  -out /private/tmp/kite-access-token.env
unset REQUEST_TOKEN
source /private/tmp/kite-access-token.env
```

Do not commit or paste Kite env values, access tokens, API secrets, or
Authorization headers. The access token usually expires around 06:00 IST the
next day, so refresh it in each new trading-day session.

## Run Spot Backfill

Default scope is `NIFTY 50,NIFTY BANK,RELIANCE,HDFCBANK,INFY,TCS,ICICIBANK`
from `2025-01-01` through today.

```bash
go run ./local-rnd/kite-backfill
```

Explicit spot/index/equity scope:

```bash
go run ./local-rnd/kite-backfill \
  -symbols 'NIFTY 50,RELIANCE,HDFCBANK,INFY,TCS,ICICIBANK' \
  -from 2025-01-01 \
  -to "$(TZ=Asia/Kolkata date +%F)" \
  -delay 350ms
```

The command fetches `minute` candles in 60-calendar-day windows and stores them
as `1m`. Re-running the same command is idempotent: existing
`(instrument_id, ts, interval)` rows are skipped.

## Run Daily Fallback Backfill

Use `-daily` only for spot/index/equity daily fallback history. It is intended
for ranges such as `2018-01-01` through `2023-12-31`, where Kite `day` candles
fill the local research DB because the NSE bhavcopy parser covers the newer
format only.

```bash
go run ./local-rnd/kite-backfill \
  -symbols 'NIFTY 50,NIFTY BANK,RELIANCE,HDFCBANK,INFY,TCS,ICICIBANK' \
  -from 2018-01-01 \
  -to 2023-12-31 \
  -daily
```

Daily mode sends one Kite `day` request per symbol for the full date range and
stores rows as `1d`. Daily candle timestamps are normalized to UTC midnight for
the Kite trading date so they line up with existing `1d` candles. It cannot be
combined with `-current-fno`.

## Run Current F&O Backfill

Only currently listed Kite futures contracts can be imported. Expired intraday
F&O history is not available from Kite.

```bash
go run ./local-rnd/kite-backfill \
  -current-fno 'NIFTY26JUNFUT,BANKNIFTY26JUNFUT' \
  -from 2026-05-01 \
  -to "$(TZ=Asia/Kolkata date +%F)" \
  -delay 350ms
```

## Run Current Options Backfill

Only currently listed (non-expired) Kite options contracts can be imported.
Pass the exact Kite trading symbols — the format is `<UNDERLYING><YYMONDD><STRIKE><CE|PE>`,
for example `NIFTY26JUN24000CE` or `BANKNIFTY26JUN53000PE`.

```bash
go run ./local-rnd/kite-backfill \
  -current-options 'NIFTY26JUN24000CE,NIFTY26JUN24000PE,BANKNIFTY26JUN53000CE,BANKNIFTY26JUN53000PE' \
  -from 2026-06-01 \
  -to "$(TZ=Asia/Kolkata date +%F)" \
  -delay 350ms
```

The importer resolves each symbol in the live Kite instruments dump, validates
it is CE or PE in NFO-OPT segment and has not expired, then stores rows as `1m`
candles. Re-running is idempotent. Cannot be combined with `-daily`.

Each run snapshots the current Kite `/instruments` CSV under
`/private/tmp/kite-backfill-instruments/` so current contract tokens are
preserved locally.

## Correction Mode

Use `-replace` only for a known bad import. It requires a reason, backs up the
existing scoped rows, deletes only that instrument/date/interval range, refetches,
and writes an audit JSONL record under
`/private/tmp/kite-backfill-replacements/<run_id>/`.

```bash
go run ./local-rnd/kite-backfill \
  -symbols RELIANCE \
  -from 2025-06-10 \
  -to 2025-06-10 \
  -replace \
  -replace-reason 'bad initial import after token/session issue'
```

## Local Guardrails

The importer refuses to write unless all of these are true:

- `APP_ENV` is `development` or `test`.
- `DB_HOST` is loopback, such as `localhost` or `127.0.0.1`.
- DB URL, host, user, and name do not look like staging, production, or VPS.
- `environment_identity`, if present, has no non-empty identity value.

If this guard fails, create a fresh local database instead of trying to import
broker data into a restored staging or production database.

## Quick Checks

Coverage for Zerodha `1m` rows:

```sql
SELECT i.exchange, i.symbol, c.interval,
       MIN(c.ts) AS first_bar,
       MAX(c.ts) AS last_bar,
       COUNT(*) AS bars
FROM candles c
JOIN instruments i ON i.id = c.instrument_id
WHERE c.provider = 'zerodha_kite'
GROUP BY i.exchange, i.symbol, c.interval
ORDER BY i.exchange, i.symbol, c.interval;
```

Count Zerodha daily fallback rows:

```sql
SELECT COUNT(*) AS zerodha_daily_fallback_rows
FROM candles
WHERE provider = 'zerodha_kite' AND interval = '1d';
```

Recent importer runs:

```sql
SELECT id, provider, sync_type, status, records_processed, started_at, completed_at, error_message
FROM sync_runs
WHERE provider = 'zerodha_kite'
ORDER BY started_at DESC
LIMIT 20;
```
