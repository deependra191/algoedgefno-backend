# Kite Backfill

Local-only Zerodha Kite intraday importer for strategy validation.

This command imports Kite `minute` candles as repo interval `1m` with provider
`zerodha_kite`. It does not import Kite `day` candles. Daily `1d` data remains
owned by the existing `nse_eod` sync.

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
`request_token` query parameter. Exchange only that request token:

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

Confirm no Zerodha daily candles were inserted:

```sql
SELECT COUNT(*) AS zerodha_daily_rows
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
