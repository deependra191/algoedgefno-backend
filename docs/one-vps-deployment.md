# One-VPS deployment runbook

This runbook is for the temporary private-staging and early-production setup on one Hetzner CPX22-class VPS. Staging image deployment is automated from the main image publish workflow, and production image promotion can be triggered manually through a separate workflow that promotes the image already pinned on staging. Do not paste real secrets into Git, issue comments, screenshots, or chat.

## Scope

- Production server-side steps are executed manually by the operator.
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
GRANT CONNECT ON DATABASE algoedgefno_prod TO algoedgefno_prod_app;
GRANT USAGE ON SCHEMA public TO algoedgefno_prod_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO algoedgefno_prod_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO algoedgefno_prod_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algoedgefno_prod_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT USAGE, SELECT ON SEQUENCES TO algoedgefno_prod_app;
```

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
GRANT CONNECT ON DATABASE algoedgefno_staging TO algoedgefno_staging_app;
GRANT USAGE ON SCHEMA public TO algoedgefno_staging_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO algoedgefno_staging_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO algoedgefno_staging_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algoedgefno_staging_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
GRANT USAGE, SELECT ON SEQUENCES TO algoedgefno_staging_app;
```

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
environment, host, container, and env-file defaults; the shared deploy script reads
the app token and database name from the environment file when `APP_TOKEN` and
`DB_NAME` are not already set, and it does not print the token.

```bash
cd /opt/algoedgefno/compose
EXPECTED_IMAGE="ghcr.io/deependra191/algoedgefno-backend@sha256:<image-digest>" \
  /opt/algoedgefno/scripts/smoke-staging.sh "<commit-sha>" 12
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

## Staging deploy automation

The `Publish backend image` GitHub Actions workflow runs on pushes to `main`.
It publishes the backend image to GHCR and exports the immutable digest-qualified
image reference, then **stops** — from PR 2 it no longer auto-deploys to staging
(the `deploy-staging` job was removed). The GHCR image remains published and
visible in the workflow summary; production remains untouched.

Deployment is performed only by the manual-only `Deploy staging` workflow with an
explicit digest. It runs on the self-hosted runner and deploys staging only. It
runs staging migrations through the root-owned wrapper
before restarting staging, do not restart production services, and do not expose
app, database, JWT, or bearer-token secret files to GitHub Actions. They do not
use `rsync`, `scp`, or inbound SSH from GitHub-hosted runners, and they do not
overwrite server env files from the repository.
Do not configure `APP_SECRET_TOKEN`, database passwords, JWT secrets, GHCR tokens,
VPS passwords, or SSH private keys in this workflow.

**Branch and runner control (replaces the prior `environment:`-based
section).** This repository is on GitHub Free + private. Environment
vars/secrets, deployment branch policies, and required reviewers are
unavailable. The deploy workflows therefore do NOT declare `environment:`
at all. Branch restriction is enforced solely by explicit
`if: github.ref == 'refs/heads/main'` on every self-hosted-runner job
(deploy AND preflight). `STAGING_BASE_URL` / `PROD_BASE_URL` are sourced
from **repository variables**. Operator discipline — "provision before
manual dispatch" — is the hold mechanism, supported by the publish
workflow no longer auto-deploying.

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
`environment=staging`, and runs the full staging smoke checks including commit,
image, clean migration state, auth behavior, CORS, and log-shape checks.

The production wrapper promotes the image already pinned in
`BACKEND_STAGING_IMAGE`, confirms staging is healthy and actually running that
digest, derives the expected migration version from the image, requires staging
to report that migration version, updates only `BACKEND_PROD_IMAGE`, runs
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

**Normal operator flow after PR 2.** When a `dev → main` integration PR
merges, `publish-backend-image.yml` builds and pushes the image, then
**stops**. To deploy:

1. Operator captures the candidate image digest from the publish workflow
   run summary.
2. Operator performs Phase L0 staging-side provisioning on the shared VPS.
3. Operator manually dispatches `deploy-staging.yml` with
   `inputs.image = <digest>`.
4. After staging soaks, operator manually dispatches `deploy-production.yml`.
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

## PR 2 deployment notes (Firebase auth)

**Per-environment rollback precondition.** PR 2 deploys are gated by
`preflight_pr1_rollback_target`. The preflight reads
`/opt/algoedgefno/compose/.env` and `/version`, and asserts: image equals the
PR 1 image, commit equals the PR 1 commit, and the migration version is either
PR 1 (16) or the candidate (18).

- **PR 2 → PR 1 rollback:** PERMITTED.
- **Pre-PR-1 rollback:** PROHIBITED as soon as PR 2 has been deployed to any
  environment.
- **Migration 018 down:** PROHIBITED while any `refresh_tokens` row exists,
  including revoked rows.

**Production smoke residue.** For **post-launch** deploys, each smoke run
updates `last_login_at` for `PROD_SMOKE_UID` and inserts+revokes one
`refresh_tokens` row per run. Cleanup: the nightly
`cleanup-expired-refresh-tokens` cron. For the **launch** deploy, the mutating
smoke is disabled (`smoke_mode=launch`); the owner's §10 Step-5 sign-in creates
the FIRST production `users` row.

**Production runtime image is staging-promoted unchanged.** The runtime image
**bundles** operator scripts at `/app/scripts/` — the image digest is the trust
anchor. Host-installed scripts come from `docker create`/`docker cp` of the
candidate digest, never a `git clone` on the VPS.

**Auto-deploy from publish workflow removed.** `publish-backend-image.yml` only
publishes the image. Deployment is strictly via `deploy-staging.yml` and
`deploy-production.yml`, both `workflow_dispatch`-only.

**GitHub plan upgrade (future option, not required).** GitHub Team would add
environment-scoped vars/secrets and deployment branch policies for this private
repository. It does **not** add required-reviewer deployment protection for a
private repository.
