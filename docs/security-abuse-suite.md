# Security Abuse Suite

This runbook covers the closed-beta curl abuse checks for staging and production.
The scripts are operational tooling: they do not mutate server configuration,
restart containers, or read server-side env files.

## Prerequisites

- Run from a shell that has `curl`, `python3`, and either Docker log access or
  `journalctl` access on the VPS.
- For `--env staging` you must ALSO export the Firebase test UIDs the suite
  uses: `TEST_UID_A`, `TEST_UID_DENIED`, `TEST_UID_CONFLICT` (the same values
  provisioned by `setup-firebase-test-users` in Phase L0). A missing
  `TEST_UID_*` makes the dependent check **SKIP** — which is not a pass.
- Do not pass tokens on the command line. When the suite needs a test bearer
  header, it writes it to a chmod-600 temporary curl config file and removes it
  on exit.

Optional payload overrides, if the default staging data shape changes:

```bash
export ABUSE_TEST_STRATEGY=ma_crossover
export ABUSE_TEST_UNDERLYING=NIFTY
export ABUSE_TEST_FROM=2026-04-01
export ABUSE_TEST_TO=2026-04-30
```

Optional endpoint/container overrides:

```bash
export ABUSE_STAGING_BASE_URL=https://staging-api.algoedgefno.com
export ABUSE_PROD_BASE_URL=https://api.algoedgefno.com
export ABUSE_STAGING_CONTAINER=algoedgefno-backend-staging
export ABUSE_PROD_CONTAINER=algoedgefno-backend-prod
```

### Staging setup — export test UIDs on the VPS

```bash
# Firebase test UIDs — use the values provisioned in Phase L0; the ones below
# are the staging convention. TEST_UID_DENIED is deliberately absent from
# ALLOWED_FIREBASE_UIDS so the allowlist-denied check gets 403.
export TEST_UID_A='staging-test-uid-a'
export TEST_UID_DENIED='staging-test-uid-denied'
export TEST_UID_CONFLICT='staging-test-uid-conflict'
```

## Environment split (PR 2)

The suite runs two materially different paths:

- **`--env staging` — mutating.** Mints Firebase ID tokens (via the in-container
  `firebase-token` binary), exercises allowlist-denied and identity-conflict
  checks, and ends with the `/auth/session` burst check last. A `trap` cleans
  up the identity-conflict fixture on exit.
- **`--env prod` — read-only.** Asserts only non-mutating invariants:
  public `/config/app`, unauthenticated/invalid-token tenant `401`, and log
  redaction. It NEVER calls
  `/auth/session`, `/auth/refresh`, `/auth/logout`, never runs `firebase-token`,
  and never touches the staging-only conflict fixture. Production **smoke**
  (post-launch only) is the single documented intentional production mutation —
  the abuse suite is not.

## Normal Run

Run staging first and confirm zero failures:

```bash
scripts/security/abuse-suite.sh --env staging
```

Then run the read-only production path:

```bash
scripts/security/abuse-suite.sh --env prod
```

Each run writes a sanitized markdown report to:

```text
scratch/security-runs/YYYY-MM-DD-{env}.md
```

The report is an operational artifact and is not source-controlled.

## Identity-conflict fixture (staging only)

The cross-provider identity-conflict check needs a pre-seeded conflicting
identity. It is created by a host script, never by the suite mutating prod:

```bash
scripts/staging-only/seed-conflict-fixture.sh
```

The script is authorized by a root-owned, mode-`400` guard file
(`/opt/algoedgefno/env/firebase-staging-fixture-project-id.guard`) carrying the
approved staging Firebase project ID; it refuses to run unless the guard matches
the running staging project. It is referenced only by `abuse-suite.sh --env
staging`. `--env prod` never touches it.

## Independent Log Check

The log redaction check can be run without the abuse suite:

```bash
scripts/security/check-log-redaction.sh --since '10 minutes ago' --env staging
scripts/security/check-log-redaction.sh --since '10 minutes ago' --env prod
```

It reads Docker logs for the selected backend container first and falls back to
container-scoped `journalctl` if Docker logs are unavailable. It reports only
pattern labels and counts, never matching log lines or secret values.

For standalone raw-secret checks, pass a chmod-600 file containing
`LABEL=VALUE` lines:

```bash
scripts/security/check-log-redaction.sh --env staging --secret-file /path/to/secret-values.txt
```

## Expected Coverage

Both environments:
- `/api/v1/config/app` returns `200` without auth and contains no tenant data.
- Protected endpoint without auth returns `401` (`missing or invalid authorization header`).
- Protected endpoint with a present-but-invalid bearer returns `401`
  (`invalid or expired token`) — a distinct error body from the missing-header
  case above. The two messages are separate constants in
  `internal/middleware/auth.go`; the suite asserts each precisely.
- Recent logs contain no bearer tokens, JWT markers, Firebase ID tokens, refresh
  tokens, secret markers, DB passwords, or full DSNs.

Staging only (mutating, burst last):
- Allowlist-denied check against a verified but intentionally non-allowlisted
  Firebase UID.
- Identity-conflict check against the staging-only seeded fixture.
- `/auth/session` burst check confirms the auth route returns `429` under
  rapid repeated exchange attempts.

## Logging redaction policy

The suite and `check-log-redaction.sh` assert the redaction contract: the
`Authorization` header is logged as `Bearer [REDACTED]`; request/response bodies
for `/auth/session`, `/auth/refresh`, and `/auth/logout` are not logged; and JSON
fields `accessToken`, `refreshToken`, `firebaseIdToken` are redacted defensively
wherever they appear. Reports show pattern labels and counts only — never
matching log lines or secret values.

## Partial-outcome note (§12)

`/auth/session` performs two writes in sequence: the `users` upsert, then the
`refresh_tokens` insert. This is intentionally **non-atomic** (storage owns
transactions; the service composes storage calls). If the upsert succeeds and
the refresh-token insert fails, the user row persists with an updated
`last_login_at` but no session is issued and the handler returns `500 internal`;
Android retries and the idempotent upsert (`DO UPDATE` on `firebase_uid`) plus a
fresh token insert recovers. `last_login_at` therefore means "last successful
Firebase verification that reached the user-upsert step", not "last successful
session issuance". Acceptable for v1 (single owner, allowlist-only); revisit if
multi-tenant ships. Full rationale: plan §12.
