# One-VPS deployment runbook

This runbook is for the temporary private-staging and early-production setup on one Hetzner CPX22-class VPS. Image publishing is automatic from `main` and auto-deploys the published digest to staging; production promotion remains a manual workflow dispatch. Production promotion deploys the image already pinned on staging. Do not paste real secrets into Git, issue comments, screenshots, or chat.

## Scope

- Production server-side steps are executed manually by the operator.
- The repo provides templates only: Docker image, Compose, Caddy, and sanitized env examples.
- PostgreSQL remains private on the DB-only internal Docker network; Caddy is attached only to the proxy network and publishes ports `80` and `443`.
- Sync jobs attach to the DB network plus a separate egress network so they can reach NSE without putting PostgreSQL on the proxy network.
- Production migrations are explicit. The backend must not auto-run production migrations.
- Staging is optional, but a `main` publish will start/restart it through the auto-staging deploy job when the self-hosted runner is listening.

## Secret access model

The files under `/opt/algoedgefno/env/` are readable only by root on the host, but Docker also receives those values through `env_file`. Treat root access, Docker group access, and permission to run `docker inspect` or `docker compose exec` on this VPS as secret access. Do not grant Docker or sudo access to anyone who should not be able to read database passwords, Firebase credentials, and JWT secrets.

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
- `deploy/scripts/algoedgefno-deploy-staging.sh` -> `/usr/local/sbin/algoedgefno-deploy-staging`
- `deploy/scripts/algoedgefno-deploy-prod.sh` -> `/usr/local/sbin/algoedgefno-deploy-prod`
- `deploy/deploy.env.example` -> `/opt/algoedgefno/compose/.env`, then edit values.
- `deploy/env/prod.env.example` -> `/opt/algoedgefno/env/prod.env`, then replace placeholders.
- `deploy/env/staging.env.example` -> `/opt/algoedgefno/env/staging.env`, then replace placeholders.
- `deploy/env/migrate-prod.env.example` -> `/opt/algoedgefno/env/migrate-prod.env`, then replace placeholders.
- `deploy/env/migrate-staging.env.example` -> `/opt/algoedgefno/env/migrate-staging.env`, then replace placeholders.
- `deploy/env/postgres.env.example` -> `/opt/algoedgefno/env/postgres.env`, then replace placeholders.

Lock down env files:

```bash
sudo chown root:root /opt/algoedgefno/env/*.env /opt/algoedgefno/compose/.env
sudo chmod 600 /opt/algoedgefno/env/*.env /opt/algoedgefno/compose/.env
sudo chown root:root /usr/local/sbin/algoedgefno-deploy-staging /usr/local/sbin/algoedgefno-deploy-prod
sudo chmod 755 /usr/local/sbin/algoedgefno-deploy-staging /usr/local/sbin/algoedgefno-deploy-prod
```

Runtime Firebase service-account JSON files are the exception to the `*.env`
mode rule. They are bind-mounted into backend containers whose app process runs
as a non-root user, so they must be readable inside the container:

```bash
sudo mkdir -p /run/secrets
sudo chown root:root /run/secrets
sudo chmod 700 /run/secrets
sudo chown root:root /run/secrets/firebase-serviceaccount-*.json
sudo chmod 444 /run/secrets/firebase-serviceaccount-*.json
```

Mode `444` on the JSON is acceptable because the parent directory is
`/run/secrets`, owned by `root:root` and mode `700` on the host. Do not apply
this exception to env files, database backups, or tokens.

> **Wrapper re-copy on script change.** The `/usr/local/sbin/algoedgefno-deploy-*`
> wrappers are copies of the repo `deploy/scripts/*.sh`. Editing those scripts in
> the repo does **not** change the live wrappers. After any change, re-copy each
> script to its `/usr/local/sbin/` path (and re-apply the `chown root:root` +
> `chmod 755` above) before the next deploy, or the deploy still runs the old logic.

## Image references

Use digest-qualified image references from the `Publish backend image` workflow
summary as deployment sources of truth:

```bash
BACKEND_PROD_IMAGE=ghcr.io/deependra191/algoedgefno-backend@sha256:<production-image-digest>
BACKEND_STAGING_IMAGE=ghcr.io/deependra191/algoedgefno-backend@sha256:<staging-image-digest>
```

The workflow also publishes a commit-SHA tag for lookup/audit purposes. Do not
use mutable `latest` tags for production deploys.

Staging should receive a candidate digest first. After staging smoke checks pass,
production should later be promoted to the exact same digest through the manual
`Deploy production` workflow or a lower-level manual fallback step. Do not rebuild or republish between
staging verification and production promotion; that would create a different
artifact.

For manual fallback builds, tag with the exact Git commit SHA:

```bash
docker build \
  --build-arg APP_VERSION=0.1.0 \
  --build-arg COMMIT_SHA="$(git rev-parse HEAD)" \
  --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t ghcr.io/deependra191/algoedgefno-backend:"$(git rev-parse HEAD)" \
  .
```

The CPX22 server is `x86_64`, so images deployed to it must include `linux/amd64`. If building on an ARM laptop, publish a multi-arch image or build explicitly for `linux/amd64` with Docker Buildx.

The normal production path should pull the reviewed image from a registry.

## First database setup

Start PostgreSQL only:

```bash
cd /opt/algoedgefno/compose
docker compose up -d postgres
docker compose ps
```

Set `POSTGRES_USER` in `postgres.env` to the admin/owner role name. That
bootstrap role owns the databases and is also the migration role. Create
isolated app roles for runtime access. Run these interactively so the real
passwords are not stored in shell history:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d postgres'
```

Inside `psql`, create the production and staging app roles/databases using the real passwords from the server env files:

```sql
CREATE ROLE algoedgefno_prod_app LOGIN PASSWORD 'replace-with-real-prod-password';
CREATE DATABASE algoedgefno_prod OWNER <postgres-admin-role>;

CREATE ROLE algoedgefno_staging_app LOGIN PASSWORD 'replace-with-real-staging-password';
CREATE DATABASE algoedgefno_staging OWNER <postgres-admin-role>;
```

Do not grant either app role access to the other database. The app roles are
runtime-only roles; migrations run as the `POSTGRES_USER` admin/owner role
because DDL needs the object owner role.

The `migrate-*.env` files must use the same admin role as `postgres.env`.
Set `DB_USER` and `DB_PASSWORD` in both files to the admin user and password.

### Existing VPS ownership normalization

Older setup notes created `algoedgefno_prod` with `algoedgefno_prod_app` as the
database and table owner. That works only while migrations also run as the app
role, and it prevents the runtime role from being limited to ordinary app
queries. Normalize existing databases so the admin role owns database objects
and app roles keep only runtime access.

Take a fresh backup before changing ownership. Then, for production:

```bash
cd /opt/algoedgefno/compose

docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -v ON_ERROR_STOP=1 -c "REASSIGN OWNED BY algoedgefno_prod_app TO $POSTGRES_USER;"'

docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -v ON_ERROR_STOP=1 -c "GRANT CONNECT ON DATABASE algoedgefno_prod TO algoedgefno_prod_app; GRANT USAGE ON SCHEMA public TO algoedgefno_prod_app; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO algoedgefno_prod_app; GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO algoedgefno_prod_app;"'
```

Repeat the same pattern for staging by replacing `prod` with `staging`.

Verify ownership and runtime access after normalization:

```bash
docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "SELECT tablename, tableowner FROM pg_tables WHERE schemaname = '\''public'\'' ORDER BY tablename;"'

docker compose exec -T postgres psql \
  -U algoedgefno_prod_app \
  -d algoedgefno_prod \
  -c "SELECT COUNT(*) FROM backtest_runs;"

docker compose exec -T postgres psql \
  -U algoedgefno_prod_app \
  -d algoedgefno_prod \
  -c "ALTER TABLE backtest_runs ADD COLUMN ddl_permission_test TEXT;"
```

The final command must fail with `must be owner of table backtest_runs`. If it
succeeds, drop the test column immediately and fix ownership before deploying.

## Migrations and identity rows

Run production migrations explicitly. The `migrate-prod` service uses
`migrate-prod.env`, not the runtime `prod.env`, so DDL runs as the admin/owner
role:

```bash
cd /opt/algoedgefno/compose
docker compose --profile migrate-prod run --rm migrate-prod
```

Grant the production runtime app role DML-only access to existing and future
objects created by the admin role:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod'
```

Inside `psql`:

```sql
-- One-time backfill: cover every table that ALREADY exists in this database.
GRANT CONNECT ON DATABASE algoedgefno_prod TO algoedgefno_prod_app;
GRANT USAGE ON SCHEMA public TO algoedgefno_prod_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO algoedgefno_prod_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO algoedgefno_prod_app;

-- Durable: auto-grant the app role on every table the admin role creates LATER
-- (i.e. every future migration). Without this, a new table added by a later
-- migration (as happened with refresh_tokens in migration 018) has ZERO
-- privileges for the app role, and the first runtime query against it fails
-- with `permission denied for table <name>` -- surfacing as a generic 500.
ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algoedgefno_prod_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT USAGE, SELECT ON SEQUENCES TO algoedgefno_prod_app;
```

**Why these statements matter -- read before substituting the placeholder:**

- **`<postgres-admin-role>` MUST be the role migrations run as** -- the
  container `POSTGRES_USER` (the same `DB_USER` set in `migrate-prod.env`).
  `ALTER DEFAULT PRIVILEGES FOR ROLE <role>` only affects objects created **by
  that role**, and only **going forward** from when the statement runs. If you
  substitute the app role here, or any role other than the migration role, the
  default privileges silently apply to nothing and future migration tables stay
  ungranted. Set this at provisioning time, before the migration that creates a
  given table runs.
- **The default-privileges grant is not retroactive.** Any table created by the
  admin role *before* the `ALTER DEFAULT PRIVILEGES` statement ran is not
  covered by it -- only by the one-time `GRANT ... ON ALL TABLES ...` backfill.
  Therefore, as a safety net, **re-run the one-time `GRANT ... ON ALL TABLES IN
  SCHEMA public ...` (and the `ALL SEQUENCES` grant) after any deploy that
  introduces new tables**, in case default privileges were not in place when
  that table was created. Re-running the backfill is idempotent and harmless.
- **No sequence grants are strictly needed today** -- every table uses UUID
  primary keys, so there are no `SERIAL`/`bigserial` columns or sequences in any
  current migration. The `ON ALL SEQUENCES` / `ON SEQUENCES` lines above are
  harmless no-ops today and are included so provisioning stays correct if a
  future migration introduces a sequence. If one is ever added, the
  `GRANT USAGE, SELECT ON ALL SEQUENCES ...` backfill and the
  `ALTER DEFAULT PRIVILEGES ... GRANT USAGE, SELECT ON SEQUENCES ...` durable
  grant shown above are exactly what that sequence needs.

> **Why this lives here and not in a numbered migration:** numbered migrations
> in this repo are env-agnostic SQL applied identically across environments
> (CLAUDE.md rule 10), and no migration contains any `GRANT`/`ROLE`/`ALTER
> DEFAULT PRIVILEGES` statement. The app and admin **role names differ per
> environment** (`algoedgefno_prod_app` vs `algoedgefno_staging_app`, and the
> admin role is whatever the operator chose for `POSTGRES_USER`), so a shared
> migration cannot hardcode them. Role grants therefore belong to env-specific
> provisioning, documented here.

Set the production identity row after the first migration creates `environment_identity`:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "INSERT INTO environment_identity (id, identity) VALUES (TRUE, '\''production'\'') ON CONFLICT (id) DO UPDATE SET identity = EXCLUDED.identity;"'
```

Run staging migrations only when staging is needed. The `migrate-staging`
service uses `migrate-staging.env`, not the runtime `staging.env`:

```bash
docker compose --profile migrate-staging run --rm migrate-staging
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging'
```

Inside `psql`:

```sql
-- One-time backfill: cover every table that ALREADY exists in this database.
GRANT CONNECT ON DATABASE algoedgefno_staging TO algoedgefno_staging_app;
GRANT USAGE ON SCHEMA public TO algoedgefno_staging_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO algoedgefno_staging_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO algoedgefno_staging_app;

-- Durable: auto-grant the app role on every table the admin role creates LATER.
ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algoedgefno_staging_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT USAGE, SELECT ON SEQUENCES TO algoedgefno_staging_app;
```

The same three caveats from the production block apply verbatim:
`<postgres-admin-role>` is the migration role (`migrate-staging.env` `DB_USER` =
container `POSTGRES_USER`), `ALTER DEFAULT PRIVILEGES` is not retroactive, and the
one-time `GRANT ... ON ALL TABLES ...` backfill must be re-run after any staging
deploy that introduces new tables. Skipping the re-run was the staging
`refresh_tokens` incident: migration 018 created the table, the app role had no
privilege on it, and the first `/auth/session` returned HTTP 500
(`permission denied for table refresh_tokens` after the `users` upsert
succeeded).

Then set the staging identity row:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_staging -c "INSERT INTO environment_identity (id, identity) VALUES (TRUE, '\''staging'\'') ON CONFLICT (id) DO UPDATE SET identity = EXCLUDED.identity;"'
```

Before future production migrations, take a fresh production backup and confirm the identity row still says `production`. Use `docs/backup-restore.md` for production backup commands and restore rehearsal steps.

Timescale restore rehearsal has extra requirements beyond plain PostgreSQL:
create the `timescaledb` extension in the target database first, run
`timescaledb_pre_restore()` / `timescaledb_post_restore()`, restore with
`--no-comments`, and re-grant the staging app role access to restored `public`
tables and sequences before expecting `/ready` to pass.

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

Or copy `scripts/smoke-deploy.sh`, `scripts/smoke-staging.sh`, and
`scripts/smoke-prod.sh` to `/opt/algoedgefno/scripts/`, make them executable, and
run the scripted deploy smoke check from the VPS. The wrappers set the target
environment, host, container, and env-file defaults; the shared deploy script
reads the database name from the environment file when `DB_NAME` is not already
set.

```bash
cd /opt/algoedgefno/compose
EXPECTED_IMAGE="ghcr.io/deependra191/algoedgefno-backend@sha256:<image-digest>" \
  /opt/algoedgefno/scripts/smoke-staging.sh "<commit-sha>" 12
```

Verify app config and protected endpoints:

```bash
curl -i https://api.<domain>/api/v1/config/app
curl -i https://api.<domain>/api/v1/backtests
curl -i -H "Authorization: Bearer <invalid-token>" https://api.<domain>/api/v1/backtests
```

Expected results:

- `/health` returns `200`.
- `/ready` returns `200` only when DB connectivity and environment identity match.
- `/api/v1/config/app` returns `200` without auth.
- No-token protected tenant request returns `401`.
- Protected tenant request with an invalid token returns `401`.
- Logs include method, path, status, latency, environment, version, commit, and request ID.
- Logs do not include bearer tokens, JWTs, DB passwords, full DSNs, or Firebase secrets.
- Browser CORS response headers are absent. CORS is intentionally disabled for v1 because there is no browser client; future browser/admin CORS support should be added in a separate PR when needed.
- Backend container health is validated through Caddy/manual smoke checks for now. Compose-level backend `healthcheck` entries can be added later after the runtime image includes a small HTTP probe tool or the app exposes a dependency-free internal probe strategy.

## Staging deploy automation

The `Publish backend image` GitHub Actions workflow runs on pushes to `main`.
It publishes the backend image to GHCR and exports the immutable digest-qualified
image reference, then auto-deploys that digest to staging through the
`auto-deploy-staging` job. The staging job runs on the self-hosted runner, shares
the `vps-deploy` concurrency group with manual staging and production deploys,
and is guarded by `if: github.ref == 'refs/heads/main'`.

The auto-staging job calls the root-owned
`/usr/local/sbin/algoedgefno-deploy-staging` wrapper with the published digest.
It runs staging migrations before restarting staging only; it does not restart
production services and does not expose app, database, JWT, or bearer-token
secret files to GitHub Actions. It does not use `rsync`, `scp`, or inbound SSH
from GitHub-hosted runners, and it does not overwrite server env files from the
repository. The manual-only `Deploy staging` workflow remains available as a
fallback or recovery path with an explicit digest.
Do not configure database passwords, JWT secrets, GHCR tokens,
VPS passwords, or SSH private keys in this workflow.

**Branch and runner control (replaces the prior `environment:`-based
section).** This repository is on GitHub Free + private. Environment
vars/secrets, deployment branch policies, and required reviewers are
unavailable. The deploy workflows therefore do NOT declare `environment:`
at all. Branch restriction is enforced solely by explicit
`if: github.ref == 'refs/heads/main'` on every self-hosted-runner job
(deploy jobs). `STAGING_BASE_URL` / `PROD_BASE_URL` are sourced
from **repository variables**. Operator discipline remains the hold mechanism:
staging provisioning must be persistent or completed before merging a `dev →
main` integration PR, because the publish workflow auto-deploys staging when the
runner is listening. Production promotion remains manual-only.

Register a self-hosted GitHub Actions runner on the VPS for this repository and
give it the custom label `algoedgefno-staging`. The runner may be started only
during deployment windows or left running as a service. If it is left running,
treat the runner as a VPS entry point for any workflow that can target its
labels: keep the label unique to deployment workflows, audit that only
`Publish backend image`, `Deploy staging`, and `Deploy production` use it, and keep the runner user's
home free of readable secrets. The runner must run as a limited Unix user such
as `github-runner`, not as `root`. Do not add the runner user to the `docker`,
`sudo`, or application env-file owner groups.

To manually start or restart the runner, run it from the `github-runner` user's
home directory, not from `/root`:

```bash
sudo su - github-runner
cd ~/actions-runner
./run.sh
```

From a root shell, the equivalent one-liner is:

```bash
sudo -u github-runner -H bash -lc 'cd ~/actions-runner && ./run.sh'
```

Leave the terminal open while the runner is needed. Stop a manually started
runner with `Ctrl+C`.

To leave the runner running in the background, install it as a systemd service
after configuring it in `/home/github-runner/actions-runner`:

```bash
cd /home/github-runner/actions-runner
sudo ./svc.sh install github-runner
sudo ./svc.sh start
sudo ./svc.sh status
```

Manage the service later with:

```bash
cd /home/github-runner/actions-runner
sudo ./svc.sh stop
sudo ./svc.sh start
sudo ./svc.sh status
```

The service should run as `github-runner`, not as `root`.

Configure these GitHub **repository variables** (not environment-scoped — GitHub
Free + private repos do not support environment-scoped vars):

- `STAGING_BASE_URL` — staging API base URL, for example `https://staging-api.<domain>`.
- `PROD_BASE_URL` — production API base URL, for example `https://api.<domain>`.

The runner VPS user must have exactly two passwordless sudo capabilities:

```sudoers
github-runner ALL=(root) NOPASSWD: /usr/local/sbin/algoedgefno-deploy-staging *
github-runner ALL=(root) NOPASSWD: /usr/local/sbin/algoedgefno-deploy-prod *
```

If the runner uses a different Unix username, replace `github-runner` in the
sudoers rules with that exact username. Do not grant the runner user direct
`docker`, `docker compose`, shell, editor, or arbitrary file-copy sudo
permissions. The root-owned wrapper validates the digest and staging URL, pulls
the exact digest before changing config, updates only `BACKEND_STAGING_IMAGE`,
runs `migrate-staging` with the migration env file, restarts only
`backend-staging`, confirms the staging URL host matches `STAGING_API_HOST` from
`/opt/algoedgefno/compose/.env`, asserts `/version` reports
`environment=staging`, rejects images below
`MIN_TENANT_SCOPED_MIGRATION_VERSION`, and runs the full staging smoke checks
including commit, image, clean migration state, auth behavior, CORS, and
log-shape checks.

The production wrapper promotes the image already pinned in
`BACKEND_STAGING_IMAGE`, confirms staging is healthy and actually running that
digest, derives the expected migration version from the image, requires staging
to report that migration version, rejects images below
`MIN_TENANT_SCOPED_MIGRATION_VERSION`, updates only `BACKEND_PROD_IMAGE`, runs
`migrate-prod` with the migration env file, restarts only `backend-prod`, and
then runs the production smoke checks before confirming the running prod
container image.

The wrappers run `docker pull` as root through the narrow sudo rules. For private
GHCR packages, the VPS must already have root-owned Docker credentials that can
read `ghcr.io/deependra191/algoedgefno-backend`. Use a package-read-only token
for that Docker login. Do not put GHCR credentials in the runner user's home,
GitHub workflow secrets, or repository files.

Keep `/opt/algoedgefno/compose/.env` non-secret. It must define both
`BACKEND_PROD_IMAGE` and `BACKEND_STAGING_IMAGE`. The workflow fails if
`BACKEND_PROD_IMAGE` is missing, and it never modifies the production image
reference.

**Normal operator flow after restoring auto-staging deploy.** Before a `dev →
main` integration PR merges, ensure staging provisioning is current and the
self-hosted runner state is intentional. Host-installed deploy wrappers and
operator scripts must already be current. If the PR changes `deploy/scripts/` or
`scripts/`, stop the runner, merge/publish the image, copy the updated files
from the published digest, then use manual `deploy-staging.yml` fallback. When
the integration PR merges, `publish-backend-image.yml` builds and pushes the
image, then auto-deploys the published digest to staging. To promote production:

1. Operator confirms the auto-staging deploy passed and captures the digest from
   the publish workflow run summary.
2. After staging soaks, operator manually dispatches `deploy-production.yml`.
   `deploy-prod.sh` mechanically promotes the staging-running digest to
   production.

**Manual fallback / recovery.** Manual dispatches of `deploy-staging.yml`
and `deploy-production.yml` MUST be triggered from the `main` branch. The
workflows enforce this with `if: github.ref == 'refs/heads/main'` on every
self-hosted-runner job. A manual dispatch from `dev` would skip the guarded
jobs and produce a green workflow run that did not actually deploy anything —
silently failing open. If a recovery deploy is needed from a non-`main` ref,
the operator must first land that change on `main`.

## Production deploy automation

The `Deploy production` GitHub Actions workflow is manual-only and must be run
from `main`. It does not accept or require a manual digest by default. Instead,
it promotes the image already pinned in `BACKEND_STAGING_IMAGE` on the VPS,
which should be the exact digest that previously passed staging smoke checks.

The workflow runs on the same self-hosted runner label, is guarded by
`if: github.ref == 'refs/heads/main'` on every job (it declares no
`environment:`), and calls the root-owned
`/usr/local/sbin/algoedgefno-deploy-prod` wrapper. The wrapper validates the
configured staging and prod hosts, confirms staging is healthy and serving the
pinned staging digest, derives the expected migration version from that image,
updates only `BACKEND_PROD_IMAGE`, runs `migrate-prod` with the migration env
file, restarts only `backend-prod`, runs production smoke checks, and confirms
the running prod container image. Deploy rollback restores the previous app
image/env only; it does not run down migrations, so production migrations must
remain backward-compatible with the previous deployed image or have a documented
manual DB recovery path.

Branch restriction for production is enforced by `if: github.ref == 'refs/heads/main'` on every job in `deploy-production.yml` (no GitHub `environment:` is used).

Backlog cleanup after the self-hosted runner deployment succeeds:

- Remove unused SSH deploy environment secrets, if they were created:
  `VPS_HOST`, `VPS_DEPLOY_USER`, `VPS_SSH_PRIVATE_KEY`, `VPS_KNOWN_HOSTS`, and
  `VPS_SSH_PORT`.
- Remove any temporary GitHub Actions CIDR SSH firewall rules.
- Remove the temporary `algoedgefno-deploy` user and SSH key only after no
  active workflow depends on them.

## Post-launch deployment notes (Firebase auth)

**Minimum-safe migration guard.** After the PR 2 launch succeeds, deploys are no
longer gated on "PR 1 is still the running rollback target". The active durable
guard is the root-owned wrapper constant
`MIN_TENANT_SCOPED_MIGRATION_VERSION=16`. The wrappers derive the candidate
image migration version from `/app/migrations/*.up.sql` inside the
digest-qualified image and reject any image below the minimum before migrations
or container restarts.

This keeps the important safety property — pre-tenant-scoped images cannot be
deployed after Firebase auth has reached an environment — without requiring the
currently running service to remain PR 1 forever. Historical variables such as
`PR1_COMMIT_SHA`, `PR1_MIGRATION_VERSION`, `PR1_IMAGE_DIGEST`, and
`PR2_CANDIDATE_MIGRATION_VERSION` are rollout evidence only; post-launch deploy
workflows do not read them.

**Rollback scope.** The root-owned deploy wrappers remain the sole writers of
the compose `.env` image pins (`BACKEND_STAGING_IMAGE` / `BACKEND_PROD_IMAGE`)
and the sole deployment-time restarters of backend containers. Wrapper rollback
restores the previous digest-qualified app image pin and restarts the app
container only. It does not run down migrations, so production migrations must
remain backward-compatible with the previous deployed image or have a documented
manual database recovery path.

- **Rollback to the previous deployed image:** PERMITTED when that image's
  migration version is at least `MIN_TENANT_SCOPED_MIGRATION_VERSION`.
- **Pre-tenant-scoped rollback:** PROHIBITED as soon as PR 2 has been deployed
  to any environment.
- **Migration 018 down:** PROHIBITED while any `refresh_tokens` row exists,
  including revoked rows.

**Production smoke residue.** For **post-launch** deploys, each smoke run
updates `last_login_at` for `PROD_SMOKE_UID` and inserts+revokes one
`refresh_tokens` row per run. Cleanup: the nightly
`cleanup-expired-refresh-tokens` cron. Standard production smoke requires both
`PROD_SMOKE_UID=<smoke-uid>` and the same UID included in
`ALLOWED_FIREBASE_UIDS` in `/opt/algoedgefno/env/prod.env`. The smoke user's
Firebase Auth record must also have `emailVerified=true`; set it with
`docker compose exec -T backend-prod /app/verify-prod-smoke-user` after
recreating `backend-prod` so the command sees the updated env. For the
**launch** deploy, the mutating smoke is disabled (`smoke_mode=launch`); the
owner's §10 Step-5 sign-in creates the FIRST production `users` row.

**Production runtime image is staging-promoted unchanged.** The runtime image
**bundles** operator scripts at `/app/scripts/` — the image digest is the trust
anchor. Host-installed scripts come from `docker create`/`docker cp` of the
candidate digest, never a `git clone` on the VPS.

**Auto-staging deploy restored.** `publish-backend-image.yml` publishes the
image and auto-deploys that digest to staging under the shared `vps-deploy`
concurrency group. `deploy-staging.yml` remains a manual fallback.
`deploy-production.yml` remains `workflow_dispatch`-only.

**GitHub plan upgrade (future option, not required).** GitHub Team would add
environment-scoped vars/secrets and deployment branch policies for this private
repository. It does **not** add required-reviewer deployment protection for a
private repository.
