# One-VPS deployment runbook

This runbook is for the temporary private-staging and early-production setup on one Hetzner CPX22-class VPS. It is intentionally manual. Do not paste real secrets into Git, issue comments, screenshots, or chat.

## Scope

- Server-side steps are executed manually by the operator.
- The repo provides templates only: Docker image, Compose, Caddy, and sanitized env examples.
- PostgreSQL remains private on the DB-only internal Docker network; Caddy is attached only to the proxy network and publishes ports `80` and `443`.
- Sync jobs attach to the DB network plus a separate egress network so they can reach NSE without putting PostgreSQL on the proxy network.
- Production migrations are explicit. The backend must not auto-run production migrations.
- Staging is optional and should stay stopped unless it is being used.

## Secret access model

The files under `/opt/algoedgefno/env/` are readable only by root on the host, but Docker also receives those values through `env_file`. Treat root access, Docker group access, and permission to run `docker inspect` or `docker compose exec` on this VPS as secret access. Do not grant Docker or sudo access to anyone who should not be able to read database passwords, app bearer tokens, and JWT secrets.

## DNS

Create DNS records before starting Caddy:

- `A api.<domain>` -> VPS public IPv4.
- `A staging-api.<domain>` -> VPS public IPv4.
- Optional: matching `AAAA` records -> VPS public IPv6.

Use those exact hostnames in `/opt/algoedgefno/compose/.env` as `PROD_API_HOST` and `STAGING_API_HOST`.

The committed `deploy/Caddyfile` contains both production and staging host blocks. Keep both DNS records in place before starting Caddy so Caddy can obtain certificates for both names. If staging DNS is intentionally not ready, remove or comment out the staging host block in `/opt/algoedgefno/caddy/Caddyfile` before starting Caddy, then restore it when staging DNS exists.

## Recommended swap

The current CPX22 has enough disk but only about 4 GiB RAM and no swap. Add a small swap file as a safety buffer:

```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
echo 'vm.swappiness=10' | sudo tee /etc/sysctl.d/99-algoedgefno-swap.conf
sudo sysctl --system
```

Swap is not a substitute for Docker memory limits. If the host starts swapping regularly, reduce workload or split staging/production.

## Server layout

Create the server directories:

```bash
sudo mkdir -p /opt/algoedgefno/{compose,env,caddy,scripts,backups,logs}
sudo chown -R root:root /opt/algoedgefno
sudo chmod 755 /opt/algoedgefno /opt/algoedgefno/{compose,caddy,scripts,backups,logs}
sudo chmod 700 /opt/algoedgefno/env
```

Copy these repo files to the server:

- `deploy/docker-compose.yml` -> `/opt/algoedgefno/compose/docker-compose.yml`
- `deploy/Caddyfile` -> `/opt/algoedgefno/caddy/Caddyfile`
- `deploy/deploy.env.example` -> `/opt/algoedgefno/compose/.env`, then edit values.
- `deploy/env/prod.env.example` -> `/opt/algoedgefno/env/prod.env`, then replace placeholders.
- `deploy/env/staging.env.example` -> `/opt/algoedgefno/env/staging.env`, then replace placeholders.
- `deploy/env/postgres.env.example` -> `/opt/algoedgefno/env/postgres.env`, then replace placeholders.

Lock down env files:

```bash
sudo chown root:root /opt/algoedgefno/env/*.env /opt/algoedgefno/compose/.env
sudo chmod 600 /opt/algoedgefno/env/*.env /opt/algoedgefno/compose/.env
```

## Image tags

Use immutable image tags, preferably the exact Git commit SHA:

```bash
docker build \
  --build-arg APP_VERSION=0.1.0 \
  --build-arg COMMIT_SHA="$(git rev-parse HEAD)" \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t ghcr.io/deependra191/algoedgefno-backend:"$(git rev-parse HEAD)" \
  .
```

The CPX22 server is `x86_64`, so images deployed to it must include `linux/amd64`. If building on an ARM laptop, publish a multi-arch image or build explicitly for `linux/amd64` with Docker Buildx.

The normal production path should pull the reviewed image from a registry. Do not use mutable `latest` tags for production deploys.

## First database setup

Start PostgreSQL only:

```bash
cd /opt/algoedgefno/compose
docker compose up -d postgres
docker compose ps
```

Create isolated app users and databases. Run these interactively so the real passwords are not stored in shell history:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d postgres'
```

Inside `psql`, create the production and staging roles/databases using the real passwords from the server env files:

```sql
CREATE ROLE algoedgefno_prod_app LOGIN PASSWORD 'replace-with-real-prod-password';
CREATE DATABASE algoedgefno_prod OWNER algoedgefno_prod_app;

CREATE ROLE algoedgefno_staging_app LOGIN PASSWORD 'replace-with-real-staging-password';
CREATE DATABASE algoedgefno_staging OWNER algoedgefno_staging_app;
```

Do not grant either app role access to the other database.

## Migrations and identity rows

Run production migrations explicitly:

```bash
cd /opt/algoedgefno/compose
docker compose --profile migrate-prod run --rm migrate-prod
```

Set the production identity row after the first migration creates `environment_identity`:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "INSERT INTO environment_identity (id, identity) VALUES (TRUE, '\''production'\'') ON CONFLICT (id) DO UPDATE SET identity = EXCLUDED.identity;"'
```

Run staging migrations only when staging is needed:

```bash
docker compose --profile migrate-staging run --rm migrate-staging
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "INSERT INTO environment_identity (id, identity) VALUES (TRUE, '\''staging'\'') ON CONFLICT (id) DO UPDATE SET identity = EXCLUDED.identity;"'
```

Before future production migrations, take a fresh production backup and confirm the identity row still says `production`. Use `docs/backup-restore.md` for production backup commands and restore rehearsal steps.

## Start services

Start production:

```bash
cd /opt/algoedgefno/compose
docker compose up -d postgres backend-prod caddy
```

Start staging only when needed:

```bash
docker compose --profile staging up -d backend-staging
```

Because Caddy is always configured with a staging hostname, `https://staging-api.<domain>` will only be useful after `backend-staging` is running. If Caddy is up but staging is stopped, requests to the staging hostname may return a proxy error; that is expected.

Stop staging when done:

```bash
docker compose --profile staging stop backend-staging
```

Run sync manually (always via flock — same lock file used by cron, prevents concurrent runs):

```bash
# Staging
flock -n /opt/algoedgefno/locks/sync-staging.lock -c \
  'cd /opt/algoedgefno/compose && docker compose --profile sync-staging run --rm sync-staging'

# Production
flock -n /opt/algoedgefno/locks/sync-prod.lock -c \
  'cd /opt/algoedgefno/compose && docker compose --profile sync-prod run --rm sync-prod'
```

Never run `docker compose ... run --rm sync-*` directly — always go through the flock wrapper. Do not overlap staging and production sync windows (≥30-minute gap).

### Stale RUNNING row reconciliation

If a sync_runs row is stuck in `RUNNING` state with no live container (e.g. after a host crash or OOM kill):

1. Confirm no container is running: `docker ps | grep sync-staging` must return empty.
2. Find the stuck row:

```sql
SELECT id, status, started_at FROM sync_runs WHERE status = 'RUNNING';
```

3. Reconcile it (replace `<uuid>` with the actual id):

```sql
UPDATE sync_runs
SET status = 'FAILED',
    finished_at = NOW(),
    error_message = 'reconciled: no active container'
WHERE id = '<uuid>' AND status = 'RUNNING';
```

Never run this SQL while a container is live.

To seed production from validated staging market data instead of re-running the full historical sync, use `docs/market-data-promotion.md`. That runbook allowlists only environment-neutral market-data tables and must not be replaced with a whole-database restore.

## Smoke checks

Run these after deployment:

```bash
curl -i https://api.<domain>/health
curl -i https://api.<domain>/ready
curl -i https://api.<domain>/version
```

Run staging smoke checks only after starting `backend-staging` with `docker compose --profile staging up -d backend-staging`:

```bash
curl -i https://staging-api.<domain>/health
curl -i https://staging-api.<domain>/ready
curl -i https://staging-api.<domain>/version
```

Verify protected endpoints:

```bash
curl -i https://api.<domain>/api/v1/config/app
curl -i -H "Authorization: Bearer <production-token>" https://api.<domain>/api/v1/config/app
curl -i -H "Authorization: Bearer <staging-token>" https://api.<domain>/api/v1/config/app
```

Expected results:

- `/health` returns `200`.
- `/ready` returns `200` only when DB connectivity and environment identity match.
- No-token protected request returns `401`.
- Production API rejects the staging token.
- Logs include method, path, status, latency, environment, version, commit, and request ID.
- Logs do not include bearer tokens, JWTs, DB passwords, full DSNs, or Firebase secrets.
- Browser CORS response headers are absent. CORS is intentionally disabled for v1 because there is no browser client; future browser/admin CORS support should be added in a separate PR when needed.
- Backend container health is validated through Caddy/manual smoke checks for now. Compose-level backend `healthcheck` entries can be added later after the runtime image includes a small HTTP probe tool or the app exposes a dependency-free internal probe strategy.

## Known follow-ups

- CI image publishing and deploy automation are separate tasks after manual deployment is proven.
