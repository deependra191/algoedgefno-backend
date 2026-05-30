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
   `/run/secrets/firebase-serviceaccount-staging.json` (root-owned, mode `400`,
   not committed) — this is the bind-mount source in `deploy/docker-compose.yml`.
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
`users` row. See the launch flow below.

---

## §10 launch flow record

Fill in as each step completes:

- [ ] **Step 1** — owner signs in to Firebase on the production Android client;
      captured owner Firebase UID: `__________`
- [ ] **Step 2** — `ALLOWED_FIREBASE_UIDS=<owner-uid>` written to
      `/opt/algoedgefno/env/prod.env` before dispatch (no backend restart yet).
- [ ] **Step 3** — `deploy-production.yml` dispatched with `smoke_mode=launch`;
      preflight passed (running `/version`: PR 1 `commit_sha`, migration 16); 017+018 applied; backend-prod
      started on PR 2 image; `smoke-prod-launch.sh` green.
- [ ] **Step 5** — owner completes `/auth/session`; first `users` row created;
      captured `users.id`: `__________`
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

- [ ] Create `PROD_SMOKE_UID` in the production Firebase Console; recorded UID:
      `__________`
- [ ] Append it to `ALLOWED_FIREBASE_UIDS`; restart backend-prod.
- [ ] Switch subsequent production dispatches to `smoke_mode=standard`.
- [ ] Activation date: `__________`

Until Step 9 runs, the owner's identity is the only allowlisted production
identity AND every production dispatch must continue to use `smoke_mode=launch`.
