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

Then run production. During the PR 1 closed interval, both environments assert
that the static app token works only for config delivery and is rejected for
tenant backtest endpoints. Tenant-authenticated abuse checks are restored in
PR 2.

```bash
scripts/security/abuse-suite.sh --env prod
```

Each run writes a sanitized markdown report to:

```text
scratch/security-runs/YYYY-MM-DD-{env}.md
```

The report is an operational artifact and is not source-controlled.

## Kill-Switch Check

During the PR 1 closed interval, kill-switch validation is unavailable. Static
app tokens cannot reach tenant backtest endpoints, so an HTTP assertion cannot
distinguish `BACKTEST_ENABLED=false` from the authentication lockdown. To
prevent a false-positive report, this command fails fast:

```bash
scripts/security/abuse-suite.sh --env staging --expect-backtests-disabled
```

PR 2 must restore this check using authenticated tenant requests before an
operator treats a `503` response as kill-switch evidence.

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

- Protected endpoint without auth returns `401`.
- Protected endpoint with invalid token returns `401`.
- Production URL with staging token returns `401` when `STAGING_APP_TOKEN` is
  available during the prod run.
- Static-token requests to tenant backtest endpoints return `401` during the PR
  1 closed interval.
- Large-range, burst-submit, aggressive-poll, cross-tenant, and kill-switch
  checks are unavailable until PR 2 restores authenticated tenant requests.
- Recent logs contain no bearer tokens, JWT markers, app secret markers, DB
  passwords, or full DSNs.
