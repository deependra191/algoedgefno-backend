# Production Checklist — algoedgefno-backend

Run through this fully before every production deploy. Do not skip items.

---

## 1. Secrets & tokens (CRITICAL — do first)

- [ ] `APP_SECRET_TOKEN` is set to a strong random value (not empty, not the example value)
- [ ] `JWT_SECRET` is set to a strong random value (not `change-this-in-production`)
- [ ] Real env files live only under `/opt/algoedgefno/env/` and `/opt/algoedgefno/compose/.env` on the VPS
- [ ] Server env files are owned by root and mode `600`; `/opt/algoedgefno/env` is mode `700`
- [ ] `.env`, `prod.env`, `staging.env`, `postgres.env`, raw service-account JSON, and backup credentials are not committed
- [ ] `.env` has never been committed — verify with `git log --all -- .env`

**How to generate strong tokens:**
```bash
openssl rand -hex 32   # generates a secure 64-character hex token
```
Run this twice — once for `APP_SECRET_TOKEN`, once for `JWT_SECRET`. Never reuse them.

---

## 2. Database

- [ ] TimescaleDB is running and reachable
- [ ] All migrations have been applied — run `migrate ... up` and confirm `no change`
- [ ] `DB_USER` is a production-only app role and includes a production marker such as `prod` or `production`
- [ ] `DB_PASSWORD` is strong, unique to production, and stored only in the server-only production env file
- [ ] `DB_NAME` is a production-only database name and includes a production marker such as `prod` or `production`
- [ ] `environment_identity` returns `production`
- [ ] DB is not exposed on a public port — only accessible from the app server

---

## 3. Server config

- [ ] `APP_ENV=production` is set — this enables startup secret validation and Gin release mode
- [ ] `GIN_MODE=release` is set
- [ ] `PORT` is set correctly
- [ ] `MIGRATIONS_PATH=file:///app/migrations`
- [ ] `AUTO_MIGRATE=false`
- [ ] `DB_SSL_REQUIRED` is set explicitly per topology — `false` for the single-VPS private-Docker-network deployment, `true` for managed Postgres (Cloud SQL, RDS, Timescale Cloud, etc.). Unset defaults to `true` (fail-closed).
- [ ] Browser CORS is disabled for Android-only production; add explicit CORS later only if a browser/admin client exists
- [ ] `TRUSTED_PROXIES` is set to the private proxy range used by Caddy or left empty when no reverse proxy headers should be trusted
- [ ] `BACKEND_PROD_IMAGE` is the digest-qualified GHCR image reference that already passed staging, not `latest`
- [ ] `BACKEND_STAGING_IMAGE` is separate from `BACKEND_PROD_IMAGE` so staging candidate deploys cannot implicitly change production
- [ ] Staging deploy runner, if enabled, runs as a limited non-root user with only the `/usr/local/sbin/algoedgefno-deploy-staging *` sudo capability
- [ ] GitHub `staging` environment restricts deployment branches to protected `dev` and `main`; reviewer approval is enabled if the repository plan supports it
- [ ] Only `Publish backend image` and `Deploy staging` use the `algoedgefno-staging` self-hosted runner label
- [ ] Root Docker auth on the VPS can pull the private GHCR backend package with read-only package credentials

---

## 4. Backup & restore readiness

- [ ] A fresh production backup exists before any production migration
- [ ] Backup filename includes environment, DB name, UTC timestamp, and migration version
- [ ] The latest production backup has been restored into staging at least once
- [ ] Restored staging DB has `environment_identity=staging`
- [ ] Restore rehearsal result is recorded outside Git
- [ ] Backup and restore steps follow `docs/backup-restore.md`

---

## 5. Verify startup

- [ ] Start the server and confirm it starts without `log.Fatal` errors
- [ ] Hit `/health` endpoint and confirm `200 OK`
- [ ] Hit `/ready` endpoint and confirm `200 OK`
- [ ] Hit `/version` endpoint and confirm environment, commit, and migration version
- [ ] Hit a protected endpoint without a token — confirm `401 Unauthorized`
- [ ] Hit a protected endpoint with the correct `APP_SECRET_TOKEN` — confirm it works
- [ ] Confirm logs contain request IDs and do not contain bearer tokens, JWTs, DB passwords, or full DSNs
- [ ] Create and review a screen-by-screen smoke-test sheet before live. For each Android screen/state, list the expected test cases, identify missing/unimplemented cases first, then run proper smoke testing against the implemented flows.

---

## 6. CI/CD — GitHub Actions (CRITICAL — must be in place before go-live)

**Status: TODO — image publishing exists, but PR gate checks still need to be added.**

The following gate checks must run automatically on every PR before merge. Running them manually is not sufficient for a production codebase.

- [ ] **Create `.github/workflows/ci.yml`** covering:
  - `go build ./...` — compile check
  - `go vet ./...` — vet warnings
  - `go test ./...` — full test suite
  - `go-arch-lint` — import boundary enforcement (config already exists at `.go-arch-lint.yml`)
- [ ] Workflow triggers on `pull_request` targeting `dev` and `main`
- [ ] Branch protection on `dev` and `main` requires the CI workflow to pass before merge — enforced in GitHub repo settings, not just by convention
- [ ] Secrets required by tests (if any) are added as GitHub Actions secrets, not committed

**Why this is CRITICAL:** without branch protection + passing CI, the architectural rules in CLAUDE.md (layer boundaries, no `json:` tags on domain types, etc.) are enforced only by code review. One missed review means a rule violation ships to prod. CI makes this machine-enforced.

---

## 7. Scheduled sync (cron)

- [ ] Staging sync cron is installed and has fired cleanly for ≥5 consecutive weekdays
- [ ] Staging failure-alert cron is installed and has fired correctly at least once in a controlled test
- [ ] Logrotate config is in place and has produced at least one rotated `.1` file
- [ ] Phase 6 gating criteria in `docs/scheduled-sync-setup.md` are all satisfied before enabling the production sync cron
- [ ] Production sync cron uses a separate lock file (`/opt/algoedgefno/locks/sync-prod.lock`) and a ≥30-minute gap from the staging window

Full phased plan, operating rules, and debugging steps: **`docs/scheduled-sync-setup.md`**.

---

## 8. Before every deploy (ongoing)

- [ ] Run `go build ./...` — confirm no compile errors
- [ ] Run `go vet ./...` — confirm no vet warnings
- [ ] Confirm no real env file is being copied into Git or image builds
- [ ] Confirm the production digest being promoted exactly matches the digest that was published from `main` and passed staging smoke checks
- [ ] If schema changed — migration files are present and tested locally first
- [ ] If schema changed — fresh production backup exists before production migration
