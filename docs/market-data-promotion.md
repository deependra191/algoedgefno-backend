# Market data promotion runbook

This runbook promotes already-validated staging market data into production on the one-VPS deployment. Use it only for environment-neutral market data, not for whole-database cloning.

> **Never run this from a broker-sourced (Zerodha/AngelOne) database.** Broker data is personal-use only and must never reach staging or production. Promote only environment-neutral, licensed/allowed datasets. See `docs/data-sourcing-policy.md`.

The first supported flow is an initial production seed where production market-data tables are empty. If production already has market data, stop and design a separate merge plan.

## Scope

Allowed tables:

- `instruments`
- `candles`

Forbidden tables:

- `environment_identity`
- `schema_migrations`
- `users`
- `strategies`
- `backtest_runs`
- `sync_runs`
- Any future auth, user-owned, app-config, audit, token, or environment-owned table

Do not use a full staging database dump against production. Do not restore staging into production. Production keeps its own migrations, identity row, secrets, users, strategies, backtests, and sync history.

## Preconditions

- Staging sync is complete and validated by the Android staging app.
- Production database and role exist.
- Production migrations have already been run.
- Production `environment_identity` is set to `production`.
- Staging and production schema migration versions match.
- `backend-prod` is stopped unless you are deliberately doing this during a maintenance window.
- Production `instruments` and `candles` are empty.

All commands below run on the VPS.

```bash
cd /opt/algoedgefno/compose
```

## Preflight checks

Confirm database identities:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT identity FROM environment_identity;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT identity FROM environment_identity;"'
```

Expected:

- Staging returns `staging`.
- Production returns `production`.

Confirm migration versions match:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"'
```

Expected:

- Both databases return the same version.
- `dirty` is `false` for both.

Confirm staging coverage:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT COUNT(*) AS instruments FROM instruments;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT MIN(ts)::date AS earliest, MAX(ts)::date AS latest, COUNT(*) AS candles FROM candles;"'
```

Confirm production market-data tables are empty:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT COUNT(*) AS instruments FROM instruments;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT COUNT(*) AS candles FROM candles;"'
```

Expected:

- Production `instruments` count is `0`.
- Production `candles` count is `0`.

If either production table is not empty, do not continue with this runbook.

## Back up production first

Create a production backup before importing data, even if production is expected to be empty.

```bash
docker compose exec postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d algoedgefno_prod --format=custom --file=/tmp/prod-before-market-data.dump'
sudo docker cp algoedgefno-postgres:/tmp/prod-before-market-data.dump /opt/algoedgefno/backups/prod-before-market-data.dump
sudo chmod 600 /opt/algoedgefno/backups/prod-before-market-data.dump
```

Keep this file until production smoke checks pass and the imported data has baked.

## Export staging market data

Export `instruments` and `candles` separately.

`instruments` is a regular table, so a table-filtered logical dump is fine:

```bash
docker compose exec postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d algoedgefno_staging --format=custom --data-only --table=instruments --file=/tmp/staging-instruments.dump'
```

`candles` is a Timescale hypertable. Do not rely on a table-filtered `pg_dump` / `pg_restore`
pair for it. Export it with `\COPY` instead:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "\COPY (SELECT * FROM candles) TO '\''/tmp/staging-candles.csv'\'' CSV"'
```

Optional: copy the exports to the host for auditability:

```bash
sudo docker cp algoedgefno-postgres:/tmp/staging-instruments.dump /opt/algoedgefno/backups/staging-instruments.dump
sudo docker cp algoedgefno-postgres:/tmp/staging-candles.csv     /opt/algoedgefno/backups/staging-candles.csv
sudo chmod 600 /opt/algoedgefno/backups/staging-instruments.dump /opt/algoedgefno/backups/staging-candles.csv
```

## Import into production

If you are retrying after a failed or partial import, clear both market-data tables first:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "TRUNCATE TABLE candles, instruments;"'
```

Import `instruments` first:

```bash
docker compose exec postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d algoedgefno_prod --data-only --single-transaction /tmp/staging-instruments.dump'
```

Import `candles` second:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "\COPY candles FROM '\''/tmp/staging-candles.csv'\'' CSV"'
```

Notes:

- `--disable-triggers` is not valid for the Timescale `candles` hypertable. It fails with `hypertables do not support enabling or disabling triggers`.
- The `candles` import can take significantly longer than the export. While the `\COPY` is still running, other sessions may continue to see `0` rows until the statement commits.
- If either import fails, do not retry blindly. Check the error first. Duplicate-key errors usually mean production was not empty and this runbook is not the right tool.

## Post-import validation

Confirm production identity was not changed:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT identity FROM environment_identity;"'
```

Expected:

- Production still returns `production`.

Compare staging and production counts:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT COUNT(*) AS instruments FROM instruments;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT COUNT(*) AS instruments FROM instruments;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT MIN(ts)::date AS earliest, MAX(ts)::date AS latest, COUNT(*) AS candles FROM candles;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT MIN(ts)::date AS earliest, MAX(ts)::date AS latest, COUNT(*) AS candles FROM candles;"'
```

Expected:

- Staging and production `instruments` counts match.
- Staging and production candle earliest/latest/count match.

Confirm production-only tables were not populated from staging:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT COUNT(*) AS users FROM users; SELECT COUNT(*) AS strategies FROM strategies; SELECT COUNT(*) AS backtest_runs FROM backtest_runs; SELECT COUNT(*) AS sync_runs FROM sync_runs;"'
```

Expected:

- Counts reflect production state only. On a fresh production database these should be `0`.

## After promotion

- Start `backend-prod` only after validation passes.
- Run production `/health`, `/ready`, and `/version` smoke checks.
- Run one protected endpoint check with the production token.
- Run a small production backtest only after the production API smoke checks pass.
- Do not run `sync-prod` immediately unless you intentionally want production sync history and incremental updates to begin.

## Rollback notes

If the import fails before commit, `--single-transaction` should leave production unchanged.

If the import succeeds but validation fails, keep `backend-prod` stopped and restore from `/opt/algoedgefno/backups/prod-before-market-data.dump` only after reviewing the failure. Do not use staging data as a rollback source.

## Cleanup

Remove the temporary export files from the Postgres container after validation:

```bash
docker compose exec postgres sh -c 'rm -f /tmp/staging-instruments.dump /tmp/staging-candles.csv'
```
