# Release notes — Firebase Auth (PR 1 + PR 2)

Operational record for the Firebase auth rollout. PR 1 locked down the tenant
auth surface and added tenant scoping; PR 2 adds Firebase ID-token → backend
session exchange (`/auth/session`, `/auth/refresh`, `/auth/logout`), refresh
rotation, the allowlist, and the deployment/CI machinery.

> **Hold mechanism.** GitHub Free + private repo does not support
> required-reviewer environment gates, environment-scoped vars/secrets, or
> deployment branch policies. The deploy workflows declare no `environment:`;
> branch restriction is `if: github.ref == 'refs/heads/main'` on every
> self-hosted job. The deployment hold is **operator discipline**: provision
> before manually dispatching the workflow. The publish workflow no longer
> auto-deploys.

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
| Deploy preflight read the root-only compose `.env` | Preflight now verifies the running service over HTTP via `/version` (commit + migration) instead | [`one-vps-deployment.md`](one-vps-deployment.md) (preflight + name-drift invariant) | #105 |
| Deploy intermittently rolled back on a `502` health check | A single immediate `curl /health` after `compose up` lost the ~0.5–2 s container-init race (reverse proxy returns 502); the prod deploy had no post-restart readiness gate at all | both deploy scripts now poll readiness (`wait_for_status`, 60 s @ 1 s); wrapper-re-copy note in [`one-vps-deployment.md`](one-vps-deployment.md) | #107 |
| Abuse suite `protected-invalid-token` failed | It expected the missing-header `401` body, but a present-but-invalid bearer returns `invalid or expired token` (a distinct constant in `internal/middleware/auth.go`) | [`security-abuse-suite.md`](security-abuse-suite.md) (Expected Coverage) | #108 |
| Abuse-suite run failed on a missing `STAGING_APP_TOKEN` | The run setup — extracting the token from `staging.env` (unquoted) and the required `TEST_UID_*` exports — was undocumented | [`security-abuse-suite.md`](security-abuse-suite.md) (Staging setup) | #108 |

> **After #107 and #108 merge:** the live VPS wrappers under `/usr/local/sbin/`
> and the scripts under `/opt/algoedgefno/scripts/` are copies installed from the
> candidate image — re-provision them from the new image (or re-copy) so the
> running copies carry these fixes.

---

## Reference values

| Key | Value |
|---|---|
| `PR1_IMAGE_DIGEST` (no longer used by the preflight; slated for removal) | `sha256:70a7f15382bba2204678d6ab44cc1fd5d4220002ec91a5d148e12b1cf9b1ccaf` |
| `PR1_COMMIT_SHA` | `e9adcc05eda6e767c1a008e5fe931139df894442` |
| `PR1_MIGRATION_VERSION` | `16` |
| `PR2_CANDIDATE_MIGRATION_VERSION` | `18` |
| `CANDIDATE_IMAGE` (PR 2 digest after `publish-backend-image.yml` runs) | _capture from publish run summary: `sha256:__________`_ |

Set `PR1_COMMIT_SHA`, `PR1_MIGRATION_VERSION=16` as GitHub repository variables
after PR 1 deploys to both environments. Set
`PR2_CANDIDATE_MIGRATION_VERSION=18` when the PR 2 release is cut. The preflight
verifies the running service via `/version` (asserting `commit_sha` ==
`PR1_COMMIT_SHA` and `migration_version` is in the accepted set), so
`PR1_IMAGE_DIGEST` is no longer used and is slated for removal.

---

## L0 provisioning runbook — staging

1. PR 2 image published; record the candidate digest as `CANDIDATE_IMAGE`.
2. Place the staging Firebase service-account JSON at the host path
   `/run/secrets/firebase-serviceaccount-staging.json` (root-owned, mode `444`,
   not committed) — this is the bind-mount source in `deploy/docker-compose.yml`.
   Runtime service-account JSON needs world-read permission because the backend
   process inside the container is non-root; the host parent directory remains
   root-owned and locked down.
   In `/opt/algoedgefno/env/staging.env` set `FIREBASE_PROJECT_ID` /
   `FIREBASE_WEB_API_KEY` to staging values and
   `FIREBASE_CREDENTIALS_FILE=/run/secrets/firebase-serviceaccount-staging.json`
   (the in-container path, which equals the mount target). Staging/prod refuse to
   start if this file is unset or unreadable (`config.ValidateServerConfig`).
3. Install the root-owned, mode-`400`
   `/opt/algoedgefno/env/firebase-staging-fixture-project-id.guard` from the
   approved staging Firebase project ID.
4. The `backend-staging` / `backend-prod` credential mounts are already wired in
   `deploy/docker-compose.yml` (bind-mount source `/run/secrets/firebase-serviceaccount-<env>.json`);
   no compose edit is needed — just confirm the host file from step 2 exists.
5. Create Firebase test users against the staging project; set
   `TEST_UID_A/B/DENIED/CONFLICT` and a non-empty `ALLOWED_FIREBASE_UIDS`.
6. Install operator scripts on the host via `docker create`/`docker cp` from
   `CANDIDATE_IMAGE` (no git clone on the VPS).
7. Approve → preflight (running `/version`: PR 1 `commit_sha`, migration 16) → dispatch `deploy-staging.yml`
   with `inputs.image = CANDIDATE_IMAGE` from `main`.

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
      preflight passed (running `/version`: PR 1 `commit_sha`, migration 16); 017+018 applied; backend-prod
      started on PR 2 image; `smoke-prod-launch.sh` green.
- [x] **Step 5** — owner completes `/auth/session`; first `users` row created;
      captured `users.id`: `bf7f4c56-8ca4-4d2e-ad89-e409bc4bba17`
- [ ] **Step 6** — owner links the second provider (`linkWithCredential`); second
      `/auth/session` returned the SAME `users.id` (DO UPDATE branch, no new row).
      Result: `__________`

**Android contract:** the Android-side Firebase auth/linking contract is tracked
at `__________` (Android repo committed file or reviewed Android PR) — see
plan §9. Link it here before launch.

---

## §10.1 decision — RETAINED (owner-confirmed)

**RETAIN.** `PROD_SMOKE_UID` becomes a second allowlisted production identity.
After launch, the operator performs §10 Step 9:

- [x] Create `PROD_SMOKE_UID` in the production Firebase Console; recorded UID:
      `3wvHesrFhTNqXDCq11irroKBdw43`
- [x] Set `PROD_SMOKE_UID` in `/opt/algoedgefno/env/prod.env`, append it to
      `ALLOWED_FIREBASE_UIDS`; restart backend-prod.
- [ ] Verify standard production smoke (`/auth/session` with `PROD_SMOKE_UID`
      returns 200; pending Firebase email verification/admin update for the
      smoke user).
- [ ] Switch subsequent production dispatches to `smoke_mode=standard`.
- [ ] Activation date: `__________`

Until standard production smoke is verified and activated, production dispatches
must continue to use `smoke_mode=launch` even though `PROD_SMOKE_UID` is present
in the allowlist.
