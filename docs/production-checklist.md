# Production Checklist — algoedgefno-backend

Run through this fully before every production deploy. Do not skip items.

This file now tracks only active open items for production readiness. Completed
launch evidence and intentionally deferred items are summarized in the appendix
at the end so they do not get counted as unresolved checklist work again.

---

## 1. Secrets & tokens (CRITICAL — do first)

- [x] `JWT_SECRET` is set to a strong random value (not `change-this-in-production`)
- [x] Real env files live only under `/opt/algoedgefno/env/` and `/opt/algoedgefno/compose/.env` on the VPS
- [x] Server env files are owned by root and mode `600`; `/opt/algoedgefno/env` is mode `700`
- [x] `.env`, `prod.env`, `staging.env`, `postgres.env`, raw service-account JSON, and backup credentials are not committed
- [x] `.env` has never been committed — verify with `git log --all -- .env`

**Firebase auth — staging-side:**
- [x] `FIREBASE_PROJECT_ID`, `FIREBASE_CREDENTIALS_FILE`, `FIREBASE_WEB_API_KEY` set to **staging** values; staging service-account JSON placed in the persistent `ENV_DIR` at `/opt/algoedgefno/env/firebase-serviceaccount-staging.json` (root-owned, not committed, mode `444` so the non-root backend process can read the bind mount; parent `/opt/algoedgefno/env` is mode `700`), bind-mounted to the in-container path `/run/secrets/firebase-serviceaccount-staging.json` which `FIREBASE_CREDENTIALS_FILE` points at. Not host `/run/secrets` (tmpfs — wiped on reboot)
- [x] `ALLOWED_FIREBASE_UIDS` populated; `TEST_UID_A`, `TEST_UID_B`, `TEST_UID_DENIED`, `TEST_UID_CONFLICT` set
- [x] Root-owned, mode-`400` `/opt/algoedgefno/env/firebase-staging-fixture-project-id.guard` created from the approved staging Firebase project ID, independently of the runtime env/credential files
- [x] Removed stale `APP_SECRET_TOKEN=` from `/opt/algoedgefno/env/staging.env`; `/api/v1/config/app` is public and the backend no longer reads that secret

**Firebase auth — production-side:**
- [x] Prod Firebase values (`FIREBASE_PROJECT_ID` is a **different project** from staging), prod service-account JSON placed in the persistent `ENV_DIR` at `/opt/algoedgefno/env/firebase-serviceaccount-prod.json` (root-owned, not committed, mode `444` so the non-root backend process can read the bind mount; parent `/opt/algoedgefno/env` is mode `700`), bind-mounted to the in-container path `/run/secrets/firebase-serviceaccount-prod.json` which `FIREBASE_CREDENTIALS_FILE` points at. Not host `/run/secrets` (tmpfs — wiped on reboot)
- [x] `ALLOWED_FIREBASE_UIDS` remains non-empty for every production deploy; `config.ValidateServerConfig` rejects startup otherwise. Current production allowlist contains the owner UID and the standard-smoke UID.
- [x] Historical launch bootstrap record is understood: the owner UID was captured from a production Firebase-only Android sign-in; no backend `users` row was manually inserted. Firebase Console **Add user** is used only for the separate `PROD_SMOKE_UID`.
- [x] **No `TEST_UID_*` in `prod.env`.** The staging-only fixture at `/opt/algoedgefno/scripts/staging-only/seed-conflict-fixture.sh` is referenced only by `abuse-suite.sh --env staging` (staging and prod share one VPS)
- [x] Removed stale `APP_SECRET_TOKEN=` from `/opt/algoedgefno/env/prod.env`; `/api/v1/config/app` is public and the backend no longer reads that secret

**GitHub repo vars (active post-launch deploy semantics):**
- [x] The root-owned deploy wrappers contain
  `MIN_TENANT_SCOPED_MIGRATION_VERSION=16`. They derive the candidate image
  migration version from the digest-qualified image and reject images below this
  minimum. This permanently blocks pre-tenant-scoped images without requiring
  the currently running service to still be PR 1.
- [x] `PR1_COMMIT_SHA`, `PR1_MIGRATION_VERSION`, `PR1_IMAGE_DIGEST`, and
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

- [x] Staging provisioning is current before the `dev → main` integration PR merges
- [x] Per-environment service-account files placed (root-owned)
- [x] Staging fixture authorization file installed root-owned/mode-`400` BEFORE any fixture binary runs
- [x] Compose credential mounts updated for `backend-staging`/`backend-prod` only (not sync-*/migrate-*)
- [x] Env files updated per Phase L0
- [x] Host-installed deploy wrappers and operator scripts are current before merge. If the PR changes `deploy/scripts/` or `scripts/`, stop the self-hosted runner, merge/publish the image, copy updated files from the published digest, then use manual `deploy-staging.yml` fallback.
- [x] After publish, operator records the candidate digest as `CANDIDATE_IMAGE`
- [x] Candidate-image preflight: `/app/firebase-token`, `/app/setup-firebase-test-users`, `/app/teardown-firebase-test-users`, `/app/verify-prod-smoke-user`, `/app/scripts/smoke-deploy.sh`, `/app/scripts/smoke-staging.sh`, `/app/scripts/smoke-prod.sh`, `/app/scripts/security/abuse-suite.sh`, `/app/scripts/security/check-log-redaction.sh`, and `/app/scripts/staging-only/seed-conflict-fixture.sh` are all present and executable (image-digest trust anchor)
- [x] If auto-staging was intentionally bypassed for script refresh, operator smoke + security-gate scripts are installed on host via `docker create`/`docker cp` from the candidate image digest — **no git clone on the VPS**
- [x] Staging only: Firebase test users created against the staging project
- [x] Auto-staging deploy passes; manual `deploy-staging.yml` is reserved for fallback/recovery
- [x] Repeat for production: no test fixtures; mechanically promote the staging digest; no manual backend `users` row provisioning; production env already contains the owner UID and `PROD_SMOKE_UID`
- [x] Migration 017's inline pre-condition is the authoritative gate for "zero users rows pre-Firebase". The migrate compose service runs `/app/migrate` only. The operator MAY use any administrative SQL access from the VPS shell (e.g. `docker compose exec postgres psql -U <postgres-admin-user> -d algoedgefno_{staging,prod} -c "SELECT COUNT(*) FROM users;"`) as an optional pre-dispatch heads-up. Skipping it is acceptable; the migration's inline guard fails closed regardless. Do NOT introduce a shell-script gate that uses the migrate compose profile or the application role.

---

## 2. Database

- [x] TimescaleDB is running and reachable
- [x] All migrations have been applied — run `migrate ... up` and confirm `no change`
- [x] `DB_USER` is a production-only app role and includes a production marker such as `prod` or `production`
- [x] `DB_PASSWORD` is strong, unique to production, and stored only in the server-only production env file
- [x] `DB_NAME` is a production-only database name and includes a production marker such as `prod` or `production`
- [x] `environment_identity` returns `production`
- [x] The `algoedgefno_prod_app` role has `SELECT/INSERT/UPDATE/DELETE` on ALL `public` tables (including any table added by the latest migration, e.g. `refresh_tokens`), AND `ALTER DEFAULT PRIVILEGES FOR ROLE <migration-admin-role>` is in place so future migration tables are auto-granted — see the production launch record below and `docs/one-vps-deployment.md`. Skipping this makes the first prod `/auth/session` return HTTP 500 (`permission denied for table refresh_tokens`)
- [x] DB is not exposed on a public port — only accessible from the app server

---

## 5. Verify startup

- [x] Start the server and confirm it starts without `log.Fatal` errors
- [x] Hit `/health` endpoint and confirm `200 OK`
- [x] Hit `/ready` endpoint and confirm `200 OK`
- [x] Hit `/version` endpoint and confirm environment, commit, and migration version
- [x] Hit `/api/v1/config/app` without a token — confirm `200 OK` and no tenant-specific or dynamic user-specific data
- [x] Hit a protected endpoint without a token — confirm `401 Unauthorized`
- [x] Hit a protected endpoint with an invalid bearer token — confirm `401 Unauthorized`
- [x] Confirm logs contain request IDs and do not contain bearer tokens, JWTs, Firebase ID tokens, refresh tokens, DB passwords, or full DSNs
- [x] Run `scripts/security/abuse-suite.sh --env staging` and confirm zero failures before merging closed-beta security changes
- [x] Run the **read-only** production subset with `scripts/security/abuse-suite.sh --env prod` before first external user access — the prod path does NOT create Firebase sessions or mutate tenant data; production **smoke** (post-launch only) is the single documented intentional mutation
- [ ] Run a full infra/security sign-off before first public closed-beta access. Produce evidence, not just ticks: route auth classification, tenant `user_id` scoping audit, production quota/kill-switch env values, staging kill-switch test, recent production backup restore rehearsal, prod/staging DB isolation proof, Android staging/release URL separation, curl abuse-suite report link, firewall/SSH hardening review, deploy-runner sudo scope, log redaction, and immutable-image deploy proof.

- [ ] Create and review a screen-by-screen smoke-test sheet before live. For each Android screen/state, list the expected test cases, identify missing/unimplemented cases first, then run proper smoke testing against the implemented flows.

**§5 Firebase verify — staging:**
- [x] Smoke `firebase_project_matches` step passes
- [x] `docker compose -f /opt/algoedgefno/compose/docker-compose.yml exec -T backend-staging sh -c '/app/firebase-token --uid="$1"' sh "$TEST_UID_A"` returns an ID token
- [x] `/auth/session` with that ID token → `200`
- [x] GET `/api/v1/backtests` with the returned accessToken → `200`
- [x] GET `/api/v1/config/app` without a token → `200` and static pre-login data only
- [x] GET `/api/v1/backtests` without a token → `401`
- [x] The deleted debug-session endpoint remains absent in every environment; automated route coverage asserts this in dev/test/staging/prod.
- [x] Staging abuse suite passes (burst last)
- [x] Nightly `cleanup-expired-refresh-tokens` cron has separate `backend-staging` and `backend-prod` invocations installed on the shared VPS
- [x] Firebase Console → Authentication → Settings → "One account per email" is ENABLED in the STAGING Firebase project
- [x] Manual cross-provider convergence verification, STAGING only, on a real Android device/emulator: (a) sign in with Google for an allowlisted staging test email, capture the Firebase UID via Console; (b) sign out, sign in with Firebase email-link for the same email, confirm SAME UID; (c) repeat in the opposite order for a second allowlisted staging test email. Uses `TEST_UID_A` and `TEST_UID_B`.
- [x] **Manual cross-tenant isolation verification (one-time, with real user data via Postman or the Android client).** Sign in as two allowlisted users A and B and obtain a backend access JWT for each. Create a backtest as A, then with B's token confirm: `GET /api/v1/backtests/{A-run-id}` → `404`; `GET /api/v1/backtests/{A-run-id}/trades` → `404`; `GET /api/v1/backtests` excludes A's run; `GET /api/v1/strategies/{slug}` shows `lastBacktest: null` for a strategy only A has run. Automated coverage lives in `internal/handlers/tenant_isolation_test.go`; this manual pass confirms it once against live data, after which the test cases are the ongoing guard.

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
- [x] Android-side Firebase auth contract is documented in a TRACKED location (Android repo committed file, or reviewed Android PR) and linked from `docs/release-notes-firebase-auth.md`
- [x] Android logout flow is verified on a production-configured build: app calls backend `/auth/logout`, clears local access/refresh state, and a subsequent protected request requires login again
- The retained production-smoke decision is recorded in `docs/release-notes-firebase-auth.md`: `PROD_SMOKE_UID` is a second allowlisted production identity.
- `PROD_SMOKE_UID` was created in the production Firebase Console, written to `/opt/algoedgefno/env/prod.env`, appended to `ALLOWED_FIREBASE_UIDS`, verified with `/app/verify-prod-smoke-user`, and activated for standard production smoke.

**§5 Firebase verify — production (post-launch deploys, `smoke_mode=standard`):**
- [x] `docker compose -f /opt/algoedgefno/compose/docker-compose.yml exec -T backend-prod sh -c '/app/firebase-token --uid="$1"' sh "$PROD_SMOKE_UID"` returns an ID token
- [x] `/auth/session` with that ID token → `200`; subsequent `/auth/logout` → `204`
- [x] GET `/api/v1/backtests` with the returned accessToken → `200`
- [x] GET `/api/v1/config/app` without a token → `200` and static pre-login data only; GET `/api/v1/backtests` without a token → `401`
- [x] The deleted debug-session endpoint remains absent in every environment; automated route coverage asserts this in dev/test/staging/prod.
- [x] Production read-only abuse suite passes; it does not invoke session/refresh/logout or data mutations
Security abuse-suite operating details: **`docs/security-abuse-suite.md`**.

---

## Historical / completed checkpoints

These items are not part of the active open checklist anymore. Keep them here as
status history and operational reference only.

### Backup & restore readiness

- Completed earlier in the launch process. See `docs/backup-restore.md` and the
  production launch / release-note records for the restore rehearsal evidence.

### Rollback procedure

- Completed as a live operating procedure: wrapper rollback restores the
  previous digest-qualified `BACKEND_*_IMAGE` pin and restarts the app
  container only.
- Pre-tenant-scoped images remain prohibited once PR 2 has been deployed
  anywhere (`MIN_TENANT_SCOPED_MIGRATION_VERSION=16`).
- Migration 018 down remains prohibited while any `refresh_tokens` row exists
  (including revoked rows).
- Migration 019 down is not a normal rollback path. If a manual schema rollback
  ever reaches 018, it only restores the nullable legacy `users.name` and
  `users.password_hash` columns; it does not remove Firebase identity data or
  refresh tokens.
- Migration 017 dirty-state recovery remains documented in the plan.

### CI/CD governance

- CI exists and runs on PRs. GitHub-enforced branch protection / rulesets are
  intentionally not part of the active checklist for this repo's current
  operating model.
- Any future hard merge gating should be tracked as a separate governance item,
  not as an open production-readiness blocker here.

### Scheduled sync

- Production sync gating was already satisfied before production sync was
  enabled. The detailed gating criteria live in
  `docs/scheduled-sync-setup.md` for reference.
- Staging sync cron, staging failure-alert cron, log rotation, and the prod
  sync rollout are no longer active checklist items.

### Server config

- `APP_ENV=production`, `GIN_MODE=release`, `PORT`, `MIGRATIONS_PATH`,
  `AUTO_MIGRATE=false`, `DB_SSL_REQUIRED`, `TRUSTED_PROXIES`, digest-qualified
  image pins, runner sudo scope, branch restriction, repository variables, and
  GHCR pull auth are all already configured and working.

### Before every deploy

- This remains an operating rule, not a one-time checklist item. Follow the
  build/vet, image-digest, and schema/backup runbook steps before each deploy.

### Monitoring & alerting

- `/opt/algoedgefno/env/healthchecks.env` exists and the active Healthchecks.io
  checks are configured and green.
- The VPS health meta-check and sync alerting are live.
- `backend-prod` and `backend-staging` declare Docker Compose `healthcheck`
  entries (`/app/healthcheck` → local `GET /ready`); `docker compose ps` reports
  `healthy`/`unhealthy` for steady-state local container health. This is a
  VPS-local signal only — off-host alerting still belongs to Healthchecks.io.
  See `docs/one-vps-deployment.md`.
- HTTP probes 1–3 and backup cron check 7 remain deferred to
  `docs/post-beta-checklist.md`.

### Deferred post-PR2 cleanup

- The main post-PR2 cleanup candidate is still tracked separately in
  `scratch/firebase-auth-plan-v2.md` §18. It is intentionally not part of the
  active production-readiness gate.
- Current cleanup candidate: tenant identity route middleware cleanup
  (`RequireUserIdentity()` / `mustUserID()`), which would turn repeated
  handler-local identity checks into a route invariant for tenant routes.
- Any future post-PR2 cleanup items should be tracked in the scratch plan or a
  dedicated follow-up doc, not added back into the active checklist unless they
  become a production gate.
