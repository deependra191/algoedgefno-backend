# Release notes — Firebase Auth (PR 1 + PR 2)

Operational record for the Firebase auth rollout. PR 1 locked down the tenant
auth surface and added tenant scoping; PR 2 adds Firebase ID-token → backend
session exchange (`/auth/session`, `/auth/refresh`, `/auth/logout`), refresh
rotation, the allowlist, and the deployment/CI machinery.

> **Hold mechanism.** GitHub Free + private repo does not support
> required-reviewer environment gates, environment-scoped vars/secrets, or
> deployment branch policies. The deploy workflows declare no `environment:`;
> branch restriction is `if: github.ref == 'refs/heads/main'` on every
> self-hosted job. The deployment hold is **operator discipline**: keep staging
> provisioning current before merging `dev → main`, because publish auto-deploys
> staging when the self-hosted runner is listening. Production promotion remains
> manual-only.

> **Public error rename.** The 409 surface for identity conflicts is
> `{"error":"identity_conflict"}`. Android error mapping must use
> `identity_conflict`. Recommendation: do NOT auto-retry on this error; surface
> a "Sign-in failed, contact support" message.

---

## Staging rollout fixes (PR 2)

Issues found and fixed while bringing PR 2 up on staging (2026-05-30 / 05-31).
Each row links to the authoritative runbook and the PR; details are not
duplicated here.

| Issue | Root cause | Authoritative doc | PR |
|---|---|---|---|
| `/auth/session` → `500` on first login | The new `refresh_tokens` table (migration 018) had no grant for the runtime app role — migrations run as the admin/owner role and nothing auto-grants later-created tables | [`one-vps-deployment.md`](one-vps-deployment.md) (role provisioning: `GRANT … ON ALL TABLES` + `ALTER DEFAULT PRIVILEGES`), [`production-checklist.md`](production-checklist.md) §10 Step 4 | #106 |
| Deploy preflight read the root-only compose `.env` | The launch preflight was moved to `/version`; after launch, deploy workflows use the durable minimum-safe migration guard instead | [`one-vps-deployment.md`](one-vps-deployment.md) (post-launch deployment notes) | #105 |
| Deploy intermittently rolled back on a `502` health check | A single immediate `curl /health` after `compose up` lost the ~0.5–2 s container-init race (reverse proxy returns 502); the prod deploy had no post-restart readiness gate at all | both deploy scripts now poll readiness (`wait_for_status`, 60 s @ 1 s); wrapper-re-copy note in [`one-vps-deployment.md`](one-vps-deployment.md) | #107 |
| Abuse suite `protected-invalid-token` failed | It expected the missing-header `401` body, but a present-but-invalid bearer returns `invalid or expired token` (a distinct constant in `internal/middleware/auth.go`) | [`security-abuse-suite.md`](security-abuse-suite.md) (Expected Coverage) | #108 |
| Abuse-suite run failed on a missing `STAGING_APP_TOKEN` | The run setup — extracting the token from `staging.env` (unquoted) and the required `TEST_UID_*` exports — was undocumented | [`security-abuse-suite.md`](security-abuse-suite.md) (Staging setup) | #108 |

> **After #107 and #108 merge:** the live VPS wrappers under `/usr/local/sbin/`
> and the scripts under `/opt/algoedgefno/scripts/` are copies installed from the
> candidate image — re-provision them from the new image (or re-copy) so the
> running copies carry these fixes.
> The same re-copy rule applies to the post-launch deploy semantics PR: the
> live `/usr/local/sbin/algoedgefno-deploy-*` wrappers must be refreshed before
> the next deploy so they contain the root-owned
> `MIN_TENANT_SCOPED_MIGRATION_VERSION=16` boundary.

---

## Reference values

| Key | Value |
|---|---|
| `PR1_IMAGE_DIGEST` (historical; not read by post-launch deploy workflows) | `sha256:70a7f15382bba2204678d6ab44cc1fd5d4220002ec91a5d148e12b1cf9b1ccaf` |
| `PR1_COMMIT_SHA` | `e9adcc05eda6e767c1a008e5fe931139df894442` |
| `PR1_MIGRATION_VERSION` | `16` |
| `PR2_CANDIDATE_MIGRATION_VERSION` | `18` |
| `MIN_TENANT_SCOPED_MIGRATION_VERSION` | `16` in the root-owned deploy wrappers |
| `CANDIDATE_IMAGE` (PR 2 digest after `publish-backend-image.yml` runs) | _capture from publish run summary: `sha256:__________`_ |

Post-launch deploy workflows do not read the PR1/PR2 rollout variables. The
root-owned wrappers derive the candidate image migration from the
digest-qualified image and reject images below
`MIN_TENANT_SCOPED_MIGRATION_VERSION=16`. The PR1 and PR2 values above are
retained as historical rollout evidence only.

---

## L0 provisioning runbook — staging

1. Candidate image published from `main`; record the candidate digest as
   `CANDIDATE_IMAGE`.
2. Place the staging Firebase service-account JSON in the persistent `ENV_DIR` at
   the host path `/opt/algoedgefno/env/firebase-serviceaccount-staging.json`
   (root-owned, mode `444`, not committed) — this is the bind-mount source in
   `deploy/docker-compose.yml` (`${ENV_DIR:-/opt/algoedgefno/env}/...`). Runtime
   service-account JSON needs world-read permission because the backend process
   inside the container is non-root; the host parent directory remains root-owned
   and mode `700`. Do **not** use host `/run/secrets` — `/run` is tmpfs and is
   wiped on reboot, which would make the fail-closed backend fail to start.
   In `/opt/algoedgefno/env/staging.env` set `FIREBASE_PROJECT_ID` /
   `FIREBASE_WEB_API_KEY` to staging values and
   `FIREBASE_CREDENTIALS_FILE=/run/secrets/firebase-serviceaccount-staging.json`
   (the in-container mount target, which Docker creates inside the container).
   Staging/prod refuse to start if this file is unset or unreadable
   (`config.ValidateServerConfig`).
3. Install the root-owned, mode-`400`
   `/opt/algoedgefno/env/firebase-staging-fixture-project-id.guard` from the
   approved staging Firebase project ID.
4. The `backend-staging` / `backend-prod` credential mounts are already wired in
   `deploy/docker-compose.yml` (bind-mount source
   `${ENV_DIR:-/opt/algoedgefno/env}/firebase-serviceaccount-<env>.json`); no
   compose edit is needed — just confirm the host file from step 2 exists.
5. Create Firebase test users against the staging project; set
   `TEST_UID_A/B/DENIED/CONFLICT` and a non-empty `ALLOWED_FIREBASE_UIDS`.
6. Ensure the host-installed deploy wrappers and operator scripts are already
   current before the merge. If the PR changes `deploy/scripts/` or `scripts/`,
   stop the self-hosted runner, merge/publish the image, copy the updated files
   from the published digest, then use manual `deploy-staging.yml` fallback.
   Do not git-clone on the VPS.
7. Merge `dev → main`; `publish-backend-image.yml` publishes the image and
   auto-deploys staging. The wrapper rejects images below
   `MIN_TENANT_SCOPED_MIGRATION_VERSION=16`. Manual `deploy-staging.yml` remains
   available for fallback/recovery with `inputs.image = CANDIDATE_IMAGE` from
   `main`.

## L0 provisioning runbook — production

No test fixtures; mechanically promote the staging-running digest; no
pre-provisioning of the user row. The owner's first sign-in creates the first
backend `users` row. The owner Firebase UID is captured before dispatch by
signing in to the production Firebase project from the Android client and then
reading the resulting UID from Firebase Console → Authentication → Users.
Firebase Console **Add user** is reserved for the post-launch `PROD_SMOKE_UID`
step, not for the owner bootstrap. See the launch flow below.

---

## §10 launch flow record

Fill in as each step completes:

- [x] **Step 1** — owner signs in to Firebase on the production Android client
      using production Firebase config; no successful backend `/auth/session`
      is required yet. Captured owner Firebase UID from Authentication → Users:
      `WoUt4uqIOQPs95tFOVRwyzVdH5T2`
- [x] **Step 2** — `ALLOWED_FIREBASE_UIDS=<owner-uid>` written to
      `/opt/algoedgefno/env/prod.env` before dispatch (no backend restart yet).
- [x] **Step 3** — `deploy-production.yml` dispatched with `smoke_mode=launch`;
      minimum-safe migration guard passed; 017+018 applied; backend-prod
      started on PR 2 image; `smoke-prod-launch.sh` green.
- [x] **Step 5** — owner completes `/auth/session`; first `users` row created;
      captured `users.id`: `bf7f4c56-8ca4-4d2e-ad89-e409bc4bba17`
- [x] **Step 6** — owner links the second provider (`linkWithCredential`); second
      `/auth/session` returned the SAME `users.id` (DO UPDATE branch, no new row).
      Result: production reverse-link verification passed on 2026-06-02; Google
      and email-link providers resolve to the same Firebase identity and the
      backend session exchange converges to existing `users.id`
      `bf7f4c56-8ca4-4d2e-ad89-e409bc4bba17` without creating a duplicate row.

**Android contract:** the Android-side Firebase auth/linking/logout contract is
tracked in the Android repo at
[docs/plans/login-firebase-v1.md](</Users/deependrasingh/AndroidStudioProjects/algoedgefno-droid/docs/plans/login-firebase-v1.md:76>) —
see plan §9.

---

## §10.1 decision — RETAINED (owner-confirmed)

**RETAIN.** `PROD_SMOKE_UID` becomes a second allowlisted production identity.
After launch, the operator performs §10 Step 9:

- [x] Create `PROD_SMOKE_UID` in the production Firebase Console; recorded UID:
      `3wvHesrFhTNqXDCq11irroKBdw43`
- [x] Set `PROD_SMOKE_UID` in `/opt/algoedgefno/env/prod.env`, append it to
      `ALLOWED_FIREBASE_UIDS`; restart backend-prod.
- [x] Mark `PROD_SMOKE_UID` email verified with the production-only operator
      command:
      ```bash
      cd /opt/algoedgefno/compose
      docker compose exec -T backend-prod /app/verify-prod-smoke-user
      ```
- [x] Verify standard production smoke (`/auth/session` with `PROD_SMOKE_UID`
      returns 200; `/auth/logout` returns 204; protected endpoint smoke passes).
- [x] Switch subsequent production dispatches to `smoke_mode=standard`.
- [x] Activation date: `2026-06-02`

Standard production smoke is active. Production dispatches now default to
`smoke_mode=standard`; `smoke_mode=launch` remains available only for exceptional
bootstrap or recovery cases.
