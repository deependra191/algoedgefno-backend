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

**Firebase auth — staging-side:**
- [ ] `FIREBASE_PROJECT_ID`, `FIREBASE_CREDENTIALS_FILE`, `FIREBASE_WEB_API_KEY` set to **staging** values; staging service-account JSON placed (root-owned, not committed)
- [ ] `ALLOWED_FIREBASE_UIDS` populated; `TEST_UID_A`, `TEST_UID_B`, `TEST_UID_DENIED`, `TEST_UID_CONFLICT` set
- [ ] Root-owned, mode-`400` `/opt/algoedgefno/env/firebase-staging-fixture-project-id.guard` created from the approved staging Firebase project ID, independently of the runtime env/credential files

**Firebase auth — production-side:**
- [ ] Prod Firebase values (`FIREBASE_PROJECT_ID` is a **different project** from staging), prod service-account JSON placed
- [ ] `ALLOWED_FIREBASE_UIDS` is **non-empty at the moment `backend-prod` first starts on the PR 2 image** — `config.ValidateServerConfig` rejects startup otherwise. Launch deploy: seed the owner's Firebase UID (§10 Step 2) BEFORE dispatching `deploy-production.yml`. Post-launch deploys: append `PROD_SMOKE_UID` (§10 Step 9), subject to the §10.1 assumption.
- [ ] **No `TEST_UID_*` in `prod.env`.** The staging-only fixture at `/opt/algoedgefno/scripts/staging-only/seed-conflict-fixture.sh` is referenced only by `abuse-suite.sh --env staging` (staging and prod share one VPS)

**GitHub repo vars (set by operator after PR 1 deploys to both envs):**
- [ ] `PR1_IMAGE_DIGEST`, `PR1_COMMIT_SHA`, `PR1_MIGRATION_VERSION=16` set
- [ ] `PR2_CANDIDATE_MIGRATION_VERSION=18` set when the PR 2 release is cut

**How to generate strong tokens:**
```bash
openssl rand -hex 32   # generates a secure 64-character hex token
```
Run this twice — once for `APP_SECRET_TOKEN`, once for `JWT_SECRET`. Never reuse them.

---

## 1.5 Pre-deploy provisioning (manual, PR 2 only)

The publish workflow no longer auto-deploys, so a `workflow_dispatch` is the only path to a deployment. Complete provisioning BEFORE dispatching:

- [ ] PR 2 image published by `publish-backend-image.yml`
- [ ] Operator records the candidate digest as `CANDIDATE_IMAGE`
- [ ] Per-environment service-account files placed (root-owned)
- [ ] Staging fixture authorization file installed root-owned/mode-`400` BEFORE any fixture binary runs
- [ ] Compose credential mounts updated for `backend-staging`/`backend-prod` only (not sync-*/migrate-*)
- [ ] Env files updated per Phase L0
- [ ] Candidate-image preflight: `/app/firebase-token`, `/app/setup-firebase-test-users`, `/app/teardown-firebase-test-users`, `/app/scripts/smoke-deploy.sh`, `/app/scripts/smoke-staging.sh`, `/app/scripts/smoke-prod.sh`, `/app/scripts/security/abuse-suite.sh`, `/app/scripts/security/check-log-redaction.sh`, and `/app/scripts/staging-only/seed-conflict-fixture.sh` are all present and executable (image-digest trust anchor)
- [ ] Operator smoke + security-gate scripts installed on host via `docker create`/`docker cp` from the candidate image digest — **no git clone on the VPS**
- [ ] Staging only: Firebase test users created against the staging project
- [ ] Operator approves → preflight runs → deploy proceeds
- [ ] Repeat for production (no test fixtures; mechanically promote the staging digest; no pre-provisioning of the user row)
- [ ] Migration 017's inline pre-condition is the authoritative gate for "zero users rows pre-Firebase". The migrate compose service runs `/app/migrate` only. The operator MAY use any administrative SQL access from the VPS shell (e.g. `docker compose exec postgres psql -U <postgres-admin-user> -d algoedgefno_{staging,prod} -c "SELECT COUNT(*) FROM users;"`) as an optional pre-dispatch heads-up. Skipping it is acceptable; the migration's inline guard fails closed regardless. Do NOT introduce a shell-script gate that uses the migrate compose profile or the application role.

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
- [ ] Deploy runner, if enabled, runs as a limited non-root user with only the `/usr/local/sbin/algoedgefno-deploy-staging *` and `/usr/local/sbin/algoedgefno-deploy-prod *` sudo capabilities
- [ ] **Branch restriction is enforced by `if: github.ref == 'refs/heads/main'` on every self-hosted-runner job (deploy AND preflight), NOT by GitHub `environment:` declarations.** GitHub Free + private repo does not support environment-scoped vars/secrets, deployment branch policies, or required reviewers, so the deploy workflows declare no `environment:` at all. The hold is operator discipline: provisioning runs BEFORE a manual `workflow_dispatch`, and `publish-backend-image.yml` no longer auto-deploys.
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
- [ ] Hit a protected endpoint without a token — confirm `401 Unauthorized`
- [ ] **APP_SECRET_TOKEN split contract (permanent from PR 2):** `APP_SECRET_TOKEN` succeeds only on `/api/v1/config/app` (→ `200 OK`) and returns `401 Unauthorized` on every tenant endpoint (e.g. `/api/v1/backtests`). Tenant endpoints require a backend access JWT obtained via `/auth/session`.
- [ ] Confirm logs contain request IDs and do not contain bearer tokens, JWTs, Firebase ID tokens, refresh tokens, DB passwords, or full DSNs
- [ ] Run `scripts/security/abuse-suite.sh --env staging` and confirm zero failures before merging closed-beta security changes
- [ ] Run the **read-only** production subset with `scripts/security/abuse-suite.sh --env prod` before first external user access — the prod path does NOT create Firebase sessions or mutate tenant data; production **smoke** (post-launch only) is the single documented intentional mutation
- Kill-switch validation (`--expect-backtests-disabled`) runs against an authenticated tenant request from PR 2 onward (it failed fast during the PR 1 closed interval).

> **Historical — PR 1 closed interval, superseded by PR 2.** Before PR 2 added Firebase JWT, the static token was rejected on tenant endpoints and the tenant-authenticated abuse checks were SKIP. The contract below documents that interval. Once PR 2 deploys, the previously-SKIP items (`burst-backtest-submit`, `aggressive-result-poll`, `backtest-large-date-range`, `cross-tenant-strategy-backtest-id-lookup`) become active in the **staging** suite.

**PR 1 closed-interval abuse-suite contract** (historical; applied after PR 1 deployed, until PR 2 added Firebase JWT):
- `GET /api/v1/backtests` with the static `APP_SECRET_TOKEN` → asserted **401** (`pr1-static-token-get-backtests`).
- `POST /api/v1/backtests` with the static `APP_SECRET_TOKEN` → asserted **401** (`pr1-static-token-post-backtests`).
- `GET /api/v1/config/app` with the static `APP_SECRET_TOKEN` → still **200** (asserted by `protected-valid-token`).
- `burst-backtest-submit`, `aggressive-result-poll`, and `backtest-large-date-range` are SKIP (tenant endpoints 401 to static token; PR 2 reintroduces them via Firebase JWT).
- `cross-tenant-strategy-backtest-id-lookup` remains SKIP until PR 2 introduces Firebase tokens.
- `--expect-backtests-disabled` is rejected rather than reporting a misleading SKIP; validate the kill switch only after PR 2 restores authenticated tenant requests.
- "Abuse suite green" in the PR 1 interval means: `run_auth_checks` passes + `run_pr1_closed_interval_check` passes + all other entries are SKIP, zero failures.
- [ ] Create and review a screen-by-screen smoke-test sheet before live. For each Android screen/state, list the expected test cases, identify missing/unimplemented cases first, then run proper smoke testing against the implemented flows.

**§5 Firebase verify — staging:**
- [ ] Smoke `firebase_project_matches` step passes
- [ ] `docker compose -f /opt/algoedgefno/compose/docker-compose.yml exec -T backend-staging sh -c '/app/firebase-token --uid="$TEST_UID_A"'` returns an ID token
- [ ] `/auth/session` with that ID token → `200`
- [ ] GET `/api/v1/backtests` with the returned accessToken → `200`
- [ ] GET `/api/v1/config/app` with `APP_SECRET_TOKEN` → `200`
- [ ] GET `/api/v1/backtests` with `APP_SECRET_TOKEN` → `401`
- [ ] POST `/api/v1/auth/debug-session` → `404`
- [ ] Staging abuse suite passes (burst last)
- [ ] Nightly `cleanup-expired-refresh-tokens` cron has separate `backend-staging` and `backend-prod` invocations installed on the shared VPS
- [ ] Firebase Console → Authentication → Settings → "One account per email" is ENABLED in the STAGING Firebase project
- [ ] Manual cross-provider convergence verification, STAGING only, on a real Android device/emulator: (a) sign in with Google for an allowlisted staging test email, capture the Firebase UID via Console; (b) sign out, sign in with Firebase email-link for the same email, confirm SAME UID; (c) repeat in the opposite order for a second allowlisted staging test email. Uses `TEST_UID_A` and `TEST_UID_B`.
- [ ] **Manual cross-tenant isolation verification (one-time, with real user data via Postman or the Android client).** Sign in as two allowlisted users A and B and obtain a backend access JWT for each. Create a backtest as A, then with B's token confirm: `GET /api/v1/backtests/{A-run-id}` → `404`; `GET /api/v1/backtests/{A-run-id}/trades` → `404`; `GET /api/v1/backtests` excludes A's run; `GET /api/v1/strategies/{slug}` shows `lastBacktest: null` for a strategy only A has run. Automated coverage lives in `internal/handlers/tenant_isolation_test.go`; this manual pass confirms it once against live data, after which the test cases are the ongoing guard (the staging abuse suite checks only the static-token boundary, not cross-user isolation).

**§5 Firebase verify — production (launch deploy, `smoke_mode=launch`):**
- [ ] Pre-launch: `deploy-production.yml` dispatched with `smoke_mode=launch`; `smoke-prod-launch.sh` runs non-identity checks only; `/auth/session` is NOT invoked by automation
- [ ] Firebase Console → "One account per email" is ENABLED in the PRODUCTION Firebase project
- [ ] §10 Step 1: owner signs in to Firebase on the production Android client (Firebase only, no backend). Operator captures the resulting Firebase UID from the Console
- [ ] §10 Step 2: operator writes `ALLOWED_FIREBASE_UIDS=<owner-uid>` into `/opt/algoedgefno/env/prod.env` BEFORE dispatching `deploy-production.yml`; records the UID in `docs/release-notes-firebase-auth.md`. NO backend restart yet — backend-prod is still on PR 1
- [ ] §10 Step 3: dispatch `deploy-production.yml` with `smoke_mode=launch`. preflight passes (PR 1 image, migration 16); deploy applies 017+018; backend-prod starts for the first time on the PR 2 image; `ValidateServerConfig` accepts the non-empty allowlist; `smoke-prod-launch.sh` returns green
- [ ] §10 Step 5: owner completes `/auth/session` via the Android client. DB query confirms exactly one users row with the owner's UID. Capture `users.id` in `docs/release-notes-firebase-auth.md`
- [ ] §10 Step 6: owner links the second provider via the Android `linkWithCredential` flow. Second `/auth/session` succeeds; same `users.id` returned (DO UPDATE branch); no new row. If `403 auth_not_allowed` appears, HALT and fix upstream (Firebase Console or Android client) — do NOT allowlist the divergent UID
- [ ] Android-side Firebase auth contract is documented in a TRACKED location (Android repo committed file, or reviewed Android PR) and linked from `docs/release-notes-firebase-auth.md`
- [ ] §10.1 assumption is **RETAIN (owner-confirmed)** — recorded in `docs/release-notes-firebase-auth.md`: `PROD_SMOKE_UID` becomes a second allowlisted production identity and post-launch dispatches use `smoke_mode=standard`. (The rejected alternative would have kept production owner-only with every dispatch on `smoke_mode=launch`.)
- [ ] §10 Step 9 (only if §10.1 RETAINED): operator creates `PROD_SMOKE_UID` in the production Firebase Console, records the UID, appends it to `ALLOWED_FIREBASE_UIDS`, restarts backend-prod, and switches subsequent dispatches to `smoke_mode=standard`. Record `PROD_SMOKE_UID` and the activation date in `docs/release-notes-firebase-auth.md`. Until this runs (or permanently if §10.1 rejected), the owner is the only allowlisted production identity AND every production dispatch must keep `smoke_mode=launch`

**§5 Firebase verify — production (post-launch deploys, `smoke_mode=standard`):**
- [ ] `docker compose -f /opt/algoedgefno/compose/docker-compose.yml exec -T backend-prod sh -c '/app/firebase-token --uid="$PROD_SMOKE_UID"'` returns an ID token
- [ ] `/auth/session` with that ID token → `200`; subsequent `/auth/logout` → `204`
- [ ] GET `/api/v1/backtests` with the returned accessToken → `200`
- [ ] GET `/api/v1/config/app` with `APP_SECRET_TOKEN` → `200`; GET `/api/v1/backtests` with `APP_SECRET_TOKEN` → `401`
- [ ] POST `/api/v1/auth/debug-session` → `404`
- [ ] Production read-only abuse suite passes; it does not invoke session/refresh/logout or data mutations

**Rollback procedure (PR 2):**
- [ ] PR 2 → PR 1 rollback is PERMITTED at any time after PR 2 deploys
- [ ] Pre-PR-1 rollback is PROHIBITED once PR 2 has been deployed to any environment (preflight enforces)
- [ ] Migration 018 down is PROHIBITED while any `refresh_tokens` row exists (including revoked rows)
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
- [x] All 5 active Healthchecks.io checks are configured and green per `docs/monitoring-setup.md` Phase 1 (HTTP probes 1–3 deferred until first non-friend user — moves to Kuma on a second machine then)
- [x] `vps-health.sh` cron entry is installed and has fired at least once (`/opt/algoedgefno/logs/vps-health-cron.log` has recent entries)
- [x] At least one synthetic subsystem failure has produced a Telegram alert per Phase 4 verification
- [x] Off-host test passed: stopping the cron daemon produced an HC "no ping received" alert within the grace window

Full monitoring setup, ping URL inventory, and verification: `docs/monitoring-setup.md`.
