# Production Checklist — algoedgefno-backend

Run through this fully before every production deploy. Do not skip items.

---

## 1. Secrets & tokens (CRITICAL — do first)

- [ ] `JWT_SECRET` is set to a strong random value (not `change-this-in-production`)
- [ ] Real env files live only under `/opt/algoedgefno/env/` and `/opt/algoedgefno/compose/.env` on the VPS
- [ ] Server env files are owned by root and mode `600`; `/opt/algoedgefno/env` is mode `700`
- [ ] `.env`, `prod.env`, `staging.env`, `postgres.env`, raw service-account JSON, and backup credentials are not committed
- [ ] `.env` has never been committed — verify with `git log --all -- .env`

**Firebase auth — staging-side:**
- [ ] `FIREBASE_PROJECT_ID`, `FIREBASE_CREDENTIALS_FILE`, `FIREBASE_WEB_API_KEY` set to **staging** values; staging service-account JSON placed at `/run/secrets/firebase-serviceaccount-staging.json` (root-owned, not committed, mode `444` so the non-root backend process can read the bind mount; `/run/secrets` remains mode `700`)
- [ ] `ALLOWED_FIREBASE_UIDS` populated; `TEST_UID_A`, `TEST_UID_B`, `TEST_UID_DENIED`, `TEST_UID_CONFLICT` set
- [ ] Root-owned, mode-`400` `/opt/algoedgefno/env/firebase-staging-fixture-project-id.guard` created from the approved staging Firebase project ID, independently of the runtime env/credential files

**Firebase auth — production-side:**
- [ ] Prod Firebase values (`FIREBASE_PROJECT_ID` is a **different project** from staging), prod service-account JSON placed at `/run/secrets/firebase-serviceaccount-prod.json` (root-owned, not committed, mode `444` so the non-root backend process can read the bind mount; `/run/secrets` remains mode `700`)
- [ ] `ALLOWED_FIREBASE_UIDS` remains non-empty for every production deploy; `config.ValidateServerConfig` rejects startup otherwise. Current production allowlist contains the owner UID and the standard-smoke UID.
- [ ] Historical launch bootstrap record is understood: the owner UID was captured from a production Firebase-only Android sign-in; no backend `users` row was manually inserted. Firebase Console **Add user** is used only for the separate `PROD_SMOKE_UID`.
- [ ] **No `TEST_UID_*` in `prod.env`.** The staging-only fixture at `/opt/algoedgefno/scripts/staging-only/seed-conflict-fixture.sh` is referenced only by `abuse-suite.sh --env staging` (staging and prod share one VPS)

**GitHub repo vars (active post-launch deploy semantics):**
- [ ] The root-owned deploy wrappers contain
  `MIN_TENANT_SCOPED_MIGRATION_VERSION=16`. They derive the candidate image
  migration version from the digest-qualified image and reject images below this
  minimum. This permanently blocks pre-tenant-scoped images without requiring
  the currently running service to still be PR 1.
- [ ] `PR1_COMMIT_SHA`, `PR1_MIGRATION_VERSION`, `PR1_IMAGE_DIGEST`, and
  `PR2_CANDIDATE_MIGRATION_VERSION` are historical rollout references only.
  Post-launch deploy workflows do not read them.

**How to generate strong tokens:**
```bash
openssl rand -hex 32   # generates a secure 64-character hex token
```
Use this for `JWT_SECRET`. Never reuse it across environments.

---

## 1.5 Pre-deploy provisioning

The publish workflow auto-deploys the published `main` digest to staging when
the self-hosted runner is listening. Complete staging provisioning BEFORE merging
a `dev → main` integration PR. Production promotion remains a manual
`workflow_dispatch`.

- [ ] Staging provisioning is current before the `dev → main` integration PR merges
- [ ] Per-environment service-account files placed (root-owned)
- [ ] Staging fixture authorization file installed root-owned/mode-`400` BEFORE any fixture binary runs
- [ ] Compose credential mounts updated for `backend-staging`/`backend-prod` only (not sync-*/migrate-*)
- [ ] Env files updated per Phase L0
- [ ] Host-installed deploy wrappers and operator scripts are current before merge. If the PR changes `deploy/scripts/` or `scripts/`, stop the self-hosted runner, merge/publish the image, copy updated files from the published digest, then use manual `deploy-staging.yml` fallback.
- [ ] After publish, operator records the candidate digest as `CANDIDATE_IMAGE`
- [ ] Candidate-image preflight: `/app/firebase-token`, `/app/setup-firebase-test-users`, `/app/teardown-firebase-test-users`, `/app/verify-prod-smoke-user`, `/app/scripts/smoke-deploy.sh`, `/app/scripts/smoke-staging.sh`, `/app/scripts/smoke-prod.sh`, `/app/scripts/security/abuse-suite.sh`, `/app/scripts/security/check-log-redaction.sh`, and `/app/scripts/staging-only/seed-conflict-fixture.sh` are all present and executable (image-digest trust anchor)
- [ ] If auto-staging was intentionally bypassed for script refresh, operator smoke + security-gate scripts are installed on host via `docker create`/`docker cp` from the candidate image digest — **no git clone on the VPS**
- [ ] Staging only: Firebase test users created against the staging project
- [ ] Auto-staging deploy passes; manual `deploy-staging.yml` is reserved for fallback/recovery
- [ ] Repeat for production: no test fixtures; mechanically promote the staging digest; no manual backend `users` row provisioning; production env already contains the owner UID and `PROD_SMOKE_UID`
- [ ] Migration 017's inline pre-condition is the authoritative gate for "zero users rows pre-Firebase". The migrate compose service runs `/app/migrate` only. The operator MAY use any administrative SQL access from the VPS shell (e.g. `docker compose exec postgres psql -U <postgres-admin-user> -d algoedgefno_{staging,prod} -c "SELECT COUNT(*) FROM users;"`) as an optional pre-dispatch heads-up. Skipping it is acceptable; the migration's inline guard fails closed regardless. Do NOT introduce a shell-script gate that uses the migrate compose profile or the application role.

---

## 2. Database

- [ ] TimescaleDB is running and reachable
- [ ] All migrations have been applied — run `migrate ... up` and confirm `no change`
- [ ] `DB_USER` is a production-only app role and includes a production marker such as `prod` or `production`
- [ ] `DB_PASSWORD` is strong, unique to production, and stored only in the server-only production env file
- [ ] `DB_NAME` is a production-only database name and includes a production marker such as `prod` or `production`
- [ ] `environment_identity` returns `production`
- [ ] The `algoedgefno_prod_app` role has `SELECT/INSERT/UPDATE/DELETE` on ALL `public` tables (including any table added by the latest migration, e.g. `refresh_tokens`), AND `ALTER DEFAULT PRIVILEGES FOR ROLE <migration-admin-role>` is in place so future migration tables are auto-granted — see the production launch record below and `docs/one-vps-deployment.md`. Skipping this makes the first prod `/auth/session` return HTTP 500 (`permission denied for table refresh_tokens`)
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
- [ ] Deploy runner, if enabled, runs as a limited non-root user with only the `/usr/local/sbin/algoedgefno-deploy-staging *` and `/usr/local/sbin/algoedgefno-deploy-prod *` sudo capabilities
- [ ] **Branch restriction is enforced by `if: github.ref == 'refs/heads/main'` on every self-hosted-runner deploy job, NOT by GitHub `environment:` declarations.** GitHub Free + private repo does not support environment-scoped vars/secrets, deployment branch policies, or required reviewers, so the deploy workflows declare no `environment:` at all. The hold is operator discipline: staging provisioning runs BEFORE a `dev → main` merge that triggers publish and auto-staging deploy; production promotion remains a manual `workflow_dispatch`.
- [ ] `STAGING_BASE_URL` / `PROD_BASE_URL` are sourced from GitHub **repository variables** (not environment-scoped)
- [ ] Only `Publish backend image`, `Deploy staging`, and `Deploy production` use the `algoedgefno-staging` self-hosted runner label
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
- [ ] Hit `/api/v1/config/app` without a token — confirm `200 OK` and no tenant-specific data
- [ ] Hit a protected endpoint without a token — confirm `401 Unauthorized`
- [ ] Hit a protected endpoint with an invalid bearer token — confirm `401 Unauthorized`
- [ ] Confirm logs contain request IDs and do not contain bearer tokens, JWTs, Firebase ID tokens, refresh tokens, DB passwords, or full DSNs
- [ ] Run `scripts/security/abuse-suite.sh --env staging` and confirm zero failures before merging closed-beta security changes
- [ ] Run the **read-only** production subset with `scripts/security/abuse-suite.sh --env prod` before first external user access — the prod path does NOT create Firebase sessions or mutate tenant data; production **smoke** (post-launch only) is the single documented intentional mutation
- [ ] Run a full infra/security sign-off before first public closed-beta access. Produce evidence, not just ticks: route auth classification, tenant `user_id` scoping audit, production quota/kill-switch env values, staging kill-switch test, recent production backup restore rehearsal, prod/staging DB isolation proof, Android staging/release URL separation, curl abuse-suite report link, firewall/SSH hardening review, deploy-runner sudo scope, log redaction, and immutable-image deploy proof.

- [ ] Create and review a screen-by-screen smoke-test sheet before live. For each Android screen/state, list the expected test cases, identify missing/unimplemented cases first, then run proper smoke testing against the implemented flows.

**§5 Firebase verify — staging:**
- [ ] Smoke `firebase_project_matches` step passes
- [ ] `docker compose -f /opt/algoedgefno/compose/docker-compose.yml exec -T backend-staging sh -c '/app/firebase-token --uid="$1"' sh "$TEST_UID_A"` returns an ID token
- [ ] `/auth/session` with that ID token → `200`
- [ ] GET `/api/v1/backtests` with the returned accessToken → `200`
- [ ] GET `/api/v1/config/app` without a token → `200`
- [ ] GET `/api/v1/backtests` without a token → `401`
- [ ] The deleted debug-session endpoint remains absent in every environment; automated route coverage asserts this in dev/test/staging/prod.
- [ ] Staging abuse suite passes (burst last)
- [ ] Nightly `cleanup-expired-refresh-tokens` cron has separate `backend-staging` and `backend-prod` invocations installed on the shared VPS
- [ ] Firebase Console → Authentication → Settings → "One account per email" is ENABLED in the STAGING Firebase project
- [ ] Manual cross-provider convergence verification, STAGING only, on a real Android device/emulator: (a) sign in with Google for an allowlisted staging test email, capture the Firebase UID via Console; (b) sign out, sign in with Firebase email-link for the same email, confirm SAME UID; (c) repeat in the opposite order for a second allowlisted staging test email. Uses `TEST_UID_A` and `TEST_UID_B`.
- [ ] **Manual cross-tenant isolation verification (one-time, with real user data via Postman or the Android client).** Sign in as two allowlisted users A and B and obtain a backend access JWT for each. Create a backtest as A, then with B's token confirm: `GET /api/v1/backtests/{A-run-id}` → `404`; `GET /api/v1/backtests/{A-run-id}/trades` → `404`; `GET /api/v1/backtests` excludes A's run; `GET /api/v1/strategies/{slug}` shows `lastBacktest: null` for a strategy only A has run. Automated coverage lives in `internal/handlers/tenant_isolation_test.go`; this manual pass confirms it once against live data, after which the test cases are the ongoing guard.

**§5 Firebase verify — production launch record (completed 2026-06-02):**

This section is historical evidence for the Firebase rollout. It is not the active path for routine production deploys; use the post-launch `smoke_mode=standard` section below for current deploys.

- Launch dispatch used `smoke_mode=launch`; `smoke-prod-launch.sh` ran non-identity checks only and automation did not invoke `/auth/session`.
- Firebase Console → "One account per email" was enabled in the PRODUCTION Firebase project.
- The owner signed in to Firebase on the production Android client; the operator captured the resulting Firebase UID from Firebase Console → Authentication → Users. This was not Firebase Console "Add user" and did not create a backend `users` row.
- The operator wrote `ALLOWED_FIREBASE_UIDS=<owner-uid>` into `/opt/algoedgefno/env/prod.env` before the launch dispatch and recorded the UID in `docs/release-notes-firebase-auth.md`.
- The wrapper verified the staging-promoted image migration was at least `MIN_TENANT_SCOPED_MIGRATION_VERSION=16`; migrations 017+018 applied; `backend-prod` started on the Firebase image; `ValidateServerConfig` accepted the non-empty allowlist; `smoke-prod-launch.sh` returned green.
- Immediately after migrations 017+018 created `refresh_tokens` as the admin role, and before the owner's first `/auth/session`, the production app-role grant was applied against `algoedgefno_prod`. Skipping this grant would have made the first production `/auth/session` return HTTP 500 (`permission denied for table refresh_tokens`: the `users` upsert succeeds on the pre-existing, already-granted table, then the `refresh_tokens` INSERT is denied). The grant block remains the reference for any future privilege backfill:
  ```sql
  -- One-time backfill for tables that already exist (covers refresh_tokens from migration 018).
  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO algoedgefno_prod_app;
  GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO algoedgefno_prod_app;
  -- Durable: auto-grant the app role on every table the admin role creates in future migrations.
  -- <postgres-admin-role> MUST be the migration role (migrate-prod.env DB_USER = container POSTGRES_USER).
  ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algoedgefno_prod_app;
  ALTER DEFAULT PRIVILEGES FOR ROLE <postgres-admin-role> IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO algoedgefno_prod_app;
  ```
  If default privileges were already set at provisioning time (per `docs/one-vps-deployment.md`), re-running this backfill is idempotent and harmless. Verify with `docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d algoedgefno_prod -c "\dp refresh_tokens"'` and confirm `algoedgefno_prod_app` has `arwd` (SELECT/INSERT/UPDATE/DELETE) access privileges.
- The owner completed `/auth/session` via the Android client. DB query confirmed exactly one users row with the owner's UID; `users.id` was captured in `docs/release-notes-firebase-auth.md`.
- The owner linked the second provider via the Android `linkWithCredential` flow. Second `/auth/session` succeeded; same `users.id` returned (DO UPDATE branch); no new row.
- [ ] Android-side Firebase auth contract is documented in a TRACKED location (Android repo committed file, or reviewed Android PR) and linked from `docs/release-notes-firebase-auth.md`
- [ ] Android logout flow is verified on a production-configured build: app calls backend `/auth/logout`, clears local access/refresh state, and a subsequent protected request requires login again
- The retained production-smoke decision is recorded in `docs/release-notes-firebase-auth.md`: `PROD_SMOKE_UID` is a second allowlisted production identity.
- `PROD_SMOKE_UID` was created in the production Firebase Console, written to `/opt/algoedgefno/env/prod.env`, appended to `ALLOWED_FIREBASE_UIDS`, verified with `/app/verify-prod-smoke-user`, and activated for standard production smoke.

**§5 Firebase verify — production (post-launch deploys, `smoke_mode=standard`):**
- [ ] `docker compose -f /opt/algoedgefno/compose/docker-compose.yml exec -T backend-prod sh -c '/app/firebase-token --uid="$1"' sh "$PROD_SMOKE_UID"` returns an ID token
- [ ] `/auth/session` with that ID token → `200`; subsequent `/auth/logout` → `204`
- [ ] GET `/api/v1/backtests` with the returned accessToken → `200`
- [ ] GET `/api/v1/config/app` without a token → `200`; GET `/api/v1/backtests` without a token → `401`
- [ ] The deleted debug-session endpoint remains absent in every environment; automated route coverage asserts this in dev/test/staging/prod.
- [ ] Production read-only abuse suite passes; it does not invoke session/refresh/logout or data mutations

**Rollback procedure (post-launch):**
- [ ] Wrapper rollback restores the previous digest-qualified `BACKEND_*_IMAGE` pin and restarts the app container only
- [ ] Pre-tenant-scoped images are PROHIBITED once PR 2 has been deployed to any environment (`MIN_TENANT_SCOPED_MIGRATION_VERSION=16` enforces)
- [ ] Migration 018 down is PROHIBITED while any `refresh_tokens` row exists (including revoked rows)
- [ ] Migration 019 down is not part of normal rollback. If a manual schema rollback ever reaches 018, it only restores nullable legacy `users.name` and `users.password_hash` columns; it does not remove Firebase identity data or refresh tokens.
- [ ] Migration 017 dirty-state recovery: see plan §15

Security abuse-suite operating details: **`docs/security-abuse-suite.md`**.

---

## 6. CI/CD — GitHub Actions (CRITICAL — must be in place before go-live)

**Status: ACCEPTED WITH MANUAL CONTROL — CI gate added; merge discipline is enforced locally until GitHub branch protection / rulesets become available for this private repo.**

The following gate checks now run automatically on every PR. Merge gating itself, however, is human discipline — see the pending item below for why.

- [x] CI workflow exists at `.github/workflows/ci.yml` and covers:
  - `go build ./...`
  - `go vet ./...`
  - `go test ./...`
  - `go-arch-lint check`
- [x] Workflow triggers on `pull_request` targeting `dev` and `main`
- [x] Local / manual branch rule is in place: do not merge PRs unless CI passes — documented in CLAUDE.md / AGENTS.md hard rule 24
- [ ] GitHub-enforced branch protection / rulesets requiring CI to pass before merge — pending. GitHub Free does not enforce branch protection or rulesets on private repositories; the repo must move to GitHub Team or Enterprise before this can be turned on. Until then, item above is the only gate.
- [ ] Secrets required by tests, if any, are added as GitHub Actions secrets, not committed

**Why this matters:** CI is now machine-run on every PR — `go build`, `go vet`, `go test`, and `go-arch-lint` all execute automatically, so failures are visible before merge. Merge enforcement, however, is currently manual: the GitHub Free plan does not enforce branch protection or rulesets on private repos, so it is the human's responsibility (per CLAUDE.md / AGENTS.md rule 24) not to click Merge on a red PR. Upgrading to GitHub Team or Enterprise would convert this from convention to machine-enforced gating — until then, the architectural rules in CLAUDE.md (layer boundaries, no `json:` tags on domain types, etc.) are enforced by code review + a visible red X, not by a hard merge block.

---

## 7. Scheduled sync (cron)

- [ ] Staging sync cron is installed and has fired cleanly for ≥5 consecutive weekdays
- [ ] Staging failure-alert cron is installed and has fired correctly at least once in a controlled test
- [ ] Logrotate config is in place and has produced at least one rotated `.1` file
- [ ] Phase 6 gating criteria in `docs/scheduled-sync-setup.md` are all satisfied before enabling the production sync cron
- [ ] Production sync cron uses a separate lock file (`/opt/algoedgefno/locks/sync-prod.lock`) and a ≥30-minute gap from the staging window
- [ ] Sync-cron HC heartbeats are wired per `docs/monitoring-setup.md` Phase 3 (Option A wrapper) so a missed sync raises a Telegram alert via Healthchecks.io

Full phased plan, operating rules, and debugging steps: **`docs/scheduled-sync-setup.md`**.

---

## 8. Before every deploy (ongoing)

- [ ] Run `go build ./...` — confirm no compile errors
- [ ] Run `go vet ./...` — confirm no vet warnings
- [ ] Confirm no real env file is being copied into Git or image builds
- [ ] Confirm the production digest being promoted exactly matches the digest that was published from `main` and passed staging smoke checks
- [ ] If schema changed — migration files are present and tested locally first
- [ ] If schema changed — fresh production backup exists before production migration

---

## 9. Monitoring & alerting

- [x] `/opt/algoedgefno/env/healthchecks.env` exists on the VPS, owned by root:root, mode 600
- [x] All 5 active Healthchecks.io checks are configured and green per `docs/monitoring-setup.md` Phase 1 (HTTP probes 1–3 deferred until first non-friend user — moves to Kuma on a second machine then; tracked in `docs/post-beta-checklist.md`)
- [x] `vps-health.sh` cron entry is installed and has fired at least once (`/opt/algoedgefno/logs/vps-health-cron.log` has recent entries)
- [x] At least one synthetic subsystem failure has produced a Telegram alert per Phase 4 verification
- [x] Off-host test passed: stopping the cron daemon produced an HC "no ping received" alert within the grace window

Full monitoring setup, ping URL inventory, and verification: `docs/monitoring-setup.md`.
