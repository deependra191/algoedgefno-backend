# Backup and restore runbook

This runbook covers logical PostgreSQL backups and restore rehearsal for the one-VPS deployment. Run these commands manually on the VPS. Do not paste real secrets, dump contents, or command output containing credentials into GitHub, chat, screenshots, or logs.

## Scope

Back up:

- Production database: `algoedgefno_prod`
- Staging database only if staging data has operational value
- Deployment metadata captured separately: image tag, commit SHA, migration version, and backup timestamp

Do not back up as a substitute for secret management. Env files under `/opt/algoedgefno/env/` remain server-only secrets and must not be copied into Git, issue comments, or screenshots.

## Backup naming

Use filenames that include environment, database name, UTC timestamp, and migration version.

Example:

```text
prod-algoedgefno_prod-v11-20260513T183000Z.dump
```

All commands below start from the Compose directory:

```bash
cd /opt/algoedgefno/compose
```

## Production backup

Confirm production identity before taking a backup:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT identity FROM environment_identity;"'
```

Expected:

- The identity is `production`.

Capture migration version:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -tAc "SELECT version FROM schema_migrations WHERE dirty = false ORDER BY version DESC LIMIT 1;"'
```

Create a compressed custom-format dump inside the Postgres container:

```bash
BACKUP_TS="$(date -u +%Y%m%dT%H%M%SZ)"
MIGRATION_VERSION="$(docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -tAc "SELECT version FROM schema_migrations WHERE dirty = false ORDER BY version DESC LIMIT 1;"' | tr -d "[:space:]")"
BACKEND_IMAGE_TAG="$(grep '^BACKEND_IMAGE=' .env | cut -d= -f2-)"
BACKUP_NAME="prod-algoedgefno_prod-v${MIGRATION_VERSION}-${BACKUP_TS}.dump"

docker compose exec postgres sh -c "pg_dump -U \"\$POSTGRES_USER\" -d algoedgefno_prod --format=custom --file=/tmp/${BACKUP_NAME}"
sudo docker cp "algoedgefno-postgres:/tmp/${BACKUP_NAME}" "/opt/algoedgefno/backups/${BACKUP_NAME}"
sudo chmod 600 "/opt/algoedgefno/backups/${BACKUP_NAME}"
```

Write non-secret metadata next to the dump:

```bash
cat <<EOF | sudo tee "/opt/algoedgefno/backups/${BACKUP_NAME}.meta" >/dev/null
environment=production
database=algoedgefno_prod
created_at_utc=${BACKUP_TS}
migration_version=${MIGRATION_VERSION}
backend_image=${BACKEND_IMAGE_TAG:-unknown}
EOF
sudo chmod 600 "/opt/algoedgefno/backups/${BACKUP_NAME}.meta"
```

Verify the backup exists and is non-empty:

```bash
sudo ls -lh "/opt/algoedgefno/backups/${BACKUP_NAME}" "/opt/algoedgefno/backups/${BACKUP_NAME}.meta"
```

Before any production migration, stop if this command does not show a fresh backup.

## Restore rehearsal into staging

Restore production only into a non-production database. This rehearsal overwrites staging data, so run it only when staging can be discarded or recreated.

Warning: after this restore, staging may contain production-like user, strategy, backtest, and market data. Treat restored staging as production-sensitive until it is wiped, recreated, or re-seeded with non-production data.

Preconditions:

- `backend-staging` is stopped.
- You have a fresh production backup in `/opt/algoedgefno/backups/`.
- You are restoring into `algoedgefno_staging`, never `algoedgefno_prod`.
- You are comfortable replacing staging data.

Stop staging API:

```bash
docker compose --profile staging stop backend-staging
```

Choose the backup file:

```bash
BACKUP_NAME="replace-with-prod-backup-name.dump"
```

Copy the backup into the Postgres container:

```bash
sudo docker cp "/opt/algoedgefno/backups/${BACKUP_NAME}" "algoedgefno-postgres:/tmp/${BACKUP_NAME}"
```

Drop and recreate the staging database. This intentionally removes existing staging data.

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '\''algoedgefno_staging'\'' AND pid <> pg_backend_pid();"'
docker compose exec postgres sh -c 'dropdb -U "$POSTGRES_USER" --if-exists algoedgefno_staging'
docker compose exec postgres sh -c 'createdb -U "$POSTGRES_USER" -O algoedgefno_staging_app algoedgefno_staging'
```

Restore the production backup into staging:

```bash
docker compose exec postgres sh -c "pg_restore -U \"\$POSTGRES_USER\" -d algoedgefno_staging --single-transaction --no-owner --role=algoedgefno_staging_app /tmp/${BACKUP_NAME}"
```

Rewrite the database identity row so the staging backend can start:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "INSERT INTO environment_identity (id, identity) VALUES (TRUE, '\''staging'\'') ON CONFLICT (id) DO UPDATE SET identity = EXCLUDED.identity;"'
```

Validate staging identity and migration version:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT identity FROM environment_identity;"'
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"'
```

Expected:

- Identity is `staging`.
- Migration version matches the restored backup metadata.
- `dirty` is `false`.

Validate critical row counts:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "SELECT COUNT(*) AS instruments FROM instruments; SELECT MIN(ts)::date AS earliest, MAX(ts)::date AS latest, COUNT(*) AS candles FROM candles; SELECT COUNT(*) AS strategies FROM strategies; SELECT COUNT(*) AS backtest_runs FROM backtest_runs;"'
```

Start staging and run smoke checks:

```bash
STAGING_API_HOST="staging-api.<domain>"
docker compose --profile staging up -d backend-staging
curl -i "https://${STAGING_API_HOST}/health"
curl -i "https://${STAGING_API_HOST}/ready"
curl -i "https://${STAGING_API_HOST}/version"
```

Expected:

- `/health` returns `200`.
- `/ready` returns `200`.
- `/version` reports `environment` as `staging`.

Record the rehearsal result outside Git:

```text
restore_rehearsed_at_utc=
operator=
backup_file=
source_database=algoedgefno_prod
target_database=algoedgefno_staging
result=
notes=
```

## After rehearsal

Choose one explicit staging posture after the restore rehearsal:

- Keep staging treated as production-sensitive, with access limited accordingly.
- Or wipe, recreate, and re-seed staging with non-production data before normal staging testing resumes.

Do not use a restored production-like staging database for casual testing as if it were ordinary staging data.

## Production rollback notes

App rollback means redeploying the previous known-good immutable image tag.

Database rollback is higher risk:

- If a migration has not run yet, do not touch the database.
- If a migration fails before changing data, inspect the failure and prefer a forward fix.
- If a destructive migration succeeds and must be reversed, restore only from a reviewed production backup.
- Do not restore staging over production.
- Do not run down migrations in production unless they were reviewed and tested for the exact failed migration.

Keep `backend-prod` stopped while reviewing any database rollback.

## Retention and cleanup

Minimum local retention before closed beta:

- Keep the latest successful pre-migration production backup.
- Keep recent daily production backups while storage allows.
- Keep at least one known-good rollback image tag.

Clean up temporary container files after copying verified backups:

```bash
docker compose exec postgres sh -c 'rm -f /tmp/*.dump'
```

Watch disk usage before and after backup jobs:

```bash
df -h / /opt/algoedgefno
docker system df
```

Treat disk usage above 85% as urgent. PostgreSQL and Docker behave badly when disk is exhausted.
