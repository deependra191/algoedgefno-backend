# Security Abuse Suite

This runbook covers the closed-beta curl abuse checks for staging and production.
The scripts are operational tooling: they do not mutate server configuration,
restart containers, or read server-side env files.

## Prerequisites

- Run from a shell that has `curl`, `python3`, and either Docker log access or
  `journalctl` access on the VPS.
- Export the target token in the operator shell. The scripts deliberately do
  NOT read server-side env files, so you extract it yourself (see "Staging
  setup" below):
  - `STAGING_APP_TOKEN` for `--env staging`
  - `PROD_APP_TOKEN` for `--env prod`
- For `--env staging` you must ALSO export the Firebase test UIDs the suite
  uses: `TEST_UID_A`, `TEST_UID_DENIED`, `TEST_UID_CONFLICT` (the same values
  provisioned by `setup-firebase-test-users` in Phase L0). A missing
  `STAGING_APP_TOKEN` fails the run immediately; a missing `TEST_UID_*` makes
  the dependent check **SKIP** — which is not a pass.
- Do not pass tokens on the command line. The scripts write bearer headers to
  chmod-600 temporary curl config files and remove them on exit.

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

### Staging setup — extract token and test UIDs on the VPS

`STAGING_APP_TOKEN` is the staging `APP_SECRET_TOKEN`. Extract it from the
compose env file (root-owned, mode 600) and strip any surrounding quotes — a
quoted value makes the `config-app-with-token` check return `401` instead of
`200`:

```bash
export STAGING_APP_TOKEN="$(sudo sed -n 's/^APP_SECRET_TOKEN=//p' \
  /opt/algoedgefno/env/staging.env | tail -1 | sed -e 's/^["'\'']//' -e 's/["'\'']$//')"

# Firebase test UIDs — use the values provisioned in Phase L0; the ones below
# are the staging convention. TEST_UID_DENIED is deliberately absent from
# ALLOWED_FIREBASE_UIDS so the allowlist-denied check gets 403.
export TEST_UID_A='staging-test-uid-a'
export TEST_UID_DENIED='staging-test-uid-denied'
export TEST_UID_CONFLICT='staging-test-uid-conflict'

# Verify the token is set and unquoted, without printing it:
case "$STAGING_APP_TOKEN" in
  \"*|*\"|\'*|*\') echo "token has surrounding quotes — re-extract" ;;
  "")             echo "token empty — extraction failed" ;;
  *)              echo "token looks clean" ;;
esac
```

## Environment split (PR 2)

The suite runs two materially different paths:

- **`--env staging` — mutating.** Mints Firebase ID tokens (via the in-container
  `firebase-token` binary), exchanges them at `/auth/session`, exercises
  tenant-authenticated abuse checks (burst submit, aggressive poll, large date
  range, cross-tenant lookup), seeds the identity-conflict fixture, and ends
  with the burst checks last. A `trap` cleans up any session/refresh artifacts
  on exit.
- **`--env prod` — read-only.** Asserts only non-mutating invariants:
  unauthenticated/invalid-token `401`, the static-token split (200 on
  `/config/app`, 401 on tenant endpoints), and log redaction. It NEVER calls
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

## Kill-Switch Check

From PR 2, kill-switch validation runs against an authenticated tenant request
(Firebase-derived backend JWT), so a `503` can be distinguished from an auth
lockdown. Run on staging:

```bash
scripts/security/abuse-suite.sh --env staging --expect-backtests-disabled
```

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
- Protected endpoint without auth returns `401` (`missing or invalid authorization header`).
- Protected endpoint with a present-but-invalid bearer returns `401`
  (`invalid or expired token`) — a distinct error body from the missing-header
  case above. The two messages are separate constants in
  `internal/middleware/auth.go`; the suite asserts each precisely.
- Production URL with staging token returns `401` when `STAGING_APP_TOKEN` is
  available during the prod run.
- Static-token split: `APP_SECRET_TOKEN` → `200` on `/api/v1/config/app`, `401`
  on tenant endpoints (permanent from PR 2).
- Recent logs contain no bearer tokens, JWT markers, Firebase ID tokens, refresh
  tokens, app secret markers, DB passwords, or full DSNs.

Staging only (mutating, burst last):
- Large-range, burst-submit, aggressive-poll, and cross-tenant lookup checks run
  against a Firebase-derived backend JWT.
- Identity-conflict check against the staging-only seeded fixture.
- Kill-switch check via `--expect-backtests-disabled`.

## Logging redaction policy

The suite and `check-log-redaction.sh` assert the redaction contract: the
`Authorization` header is logged as `Bearer [REDACTED]`; request/response bodies
for `/auth/session`, `/auth/refresh`, `/auth/logout`, and `/auth/debug-session`
are not logged; and JSON fields `accessToken`, `refreshToken`, `firebaseIdToken`
are redacted defensively wherever they appear. Reports show pattern labels and
counts only — never matching log lines or secret values.

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
