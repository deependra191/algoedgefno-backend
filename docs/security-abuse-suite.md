# Security Abuse Suite

This runbook covers the closed-beta curl abuse checks for staging and production.
The scripts are operational tooling: they do not mutate server configuration,
restart containers, or read server-side env files.

## Prerequisites

- Run from a shell that has `curl`, `python3`, and either Docker log access or
  `journalctl` access on the VPS.
- Export the target token in the operator shell:
  - `STAGING_APP_TOKEN` for `--env staging`
  - `PROD_APP_TOKEN` for `--env prod`
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

## Normal Run

Run staging first and confirm zero failures:

```bash
scripts/security/abuse-suite.sh --env staging
```

Then run production. Production intentionally excludes load cases; it verifies
auth, validation, and log redaction only.

```bash
scripts/security/abuse-suite.sh --env prod
```

Each run writes a sanitized markdown report to:

```text
scratch/security-runs/YYYY-MM-DD-{env}.md
```

The report is an operational artifact and is not source-controlled.

## Kill-Switch Check

The script never toggles `BACKTEST_ENABLED` and never restarts containers. The
operator performs the staging-only kill-switch workflow manually:

1. Set `BACKTEST_ENABLED=false` in the staging env file on the VPS.
2. Restart only the staging backend container.
3. Run:

```bash
scripts/security/abuse-suite.sh --env staging --expect-backtests-disabled
```

4. Confirm the backtest submission check returns `503` with the expected error
   JSON.
5. Restore `BACKTEST_ENABLED=true`, restart staging, and run the normal staging
   suite again:

```bash
scripts/security/abuse-suite.sh --env staging
```

## Independent Log Check

The log redaction check can be run without the abuse suite:

```bash
scripts/security/check-log-redaction.sh --since '10 minutes ago' --env staging
scripts/security/check-log-redaction.sh --since '10 minutes ago' --env prod
```

It reads Docker logs for the selected backend container first and falls back to
`journalctl` if Docker logs are unavailable. It reports only pattern labels and
counts, never matching log lines.

## Expected Coverage

- Protected endpoint without auth returns `401`.
- Protected endpoint with invalid token returns `401`.
- Production URL with staging token returns `401` when `STAGING_APP_TOKEN` is
  available during the prod run.
- Large backtest date range returns `422`.
- Staging kill-switch mode returns `503`.
- Staging burst submit and aggressive result polling produce no `5xx`.
- Cross-tenant lookup remains visible as `[skip: single-user platform]` until
  multi-user auth lands.
- Recent logs contain no bearer tokens, JWT markers, app secret markers, DB
  passwords, or full DSNs.
