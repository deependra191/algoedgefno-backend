# Scheduled VPS Sync — Staging First, Production Later

Phased plan for setting up the NSE EOD sync as a scheduled cron job on the one-VPS deployment. Staging is wired up first and observed for stability before production is gated in.

This runbook complements `docs/one-vps-deployment.md` (which documents the manual `flock`-wrapped sync invocation) by adding the cron, log rotation, and alerting layers around it.

**Execution model:** all VPS commands are run by the operator over SSH. Claude does not have server access. After each step the operator pastes back the relevant output before moving on.

---

## Phase 0 — Pre-flight (do once, before any scheduling)

### 0.1 Confirm code-side prerequisites (local, not VPS)

- `cmd/sync` runs in no-flag daily catch-up mode.
- `Config.ValidateStartupIdentity()` enforces `APP_ENV` matches the DB.
- `SYNC_ENABLED` kill switch is honored at sync startup (exits 0, not 1, when disabled — correct for cron, won't trigger false-positive failure alerts).
- `deploy/docker-compose.yml` defines `sync-staging` and `sync-prod` services under profiles `sync-staging` and `sync-prod`, with `depends_on: postgres` only — never `depends_on: backend-*`.

### 0.2 Confirm staging data state (on VPS)

- Manual staging backfill is complete through the most recent trading day.
- No `algoedgefno-sync-staging` container is currently running (`docker ps | grep sync-staging` returns empty).
- `sync_runs` has no lingering `RUNNING` rows. If one exists, do not schedule until it is reconciled (see 0.5).

### 0.3 Staging env file

On the VPS, `staging.env` must contain:

- `APP_ENV=staging`
- Correct staging DB credentials (DB user/name include a non-production marker)
- `SYNC_ENABLED=true`

No secrets in any other file. No secrets in logs.

### 0.4 Pick time slots now (do not defer)

- Staging sync: **00:15 IST Tue–Sat** (= UTC Mon–Fri at 18:45) = `45 18 * * 1-5` in UTC.
- Production sync (when later enabled): **00:45 IST Tue–Sat** (= UTC Mon–Fri at 19:15) = `15 19 * * 1-5` in UTC.
- Staging alert: **06:00 IST Tue–Sat** (= UTC Tue–Sat at 00:30) = `30 0 * * 2-6` in UTC. Must be ≥5h after the sync to give the 12h lookback window real data to see.
- 30-minute separation between staging and production sync windows; both well after NSE EOD publish window; never `00:00`.

> **Weekday-only by design.** NSE publishes bhavcopies for Mon–Fri IST trading days only. Running sync on UTC weekends produces FAILED runs (NSE 404) that pollute `sync_runs` and create alert noise. The day-of-week field (`1-5` for sync, `2-6` for alert) is what enforces this — without it the cron runs daily.

> **Server timezone is UTC** on the Hetzner VPS. Crontab times are in UTC. Convert IST → UTC by subtracting 5h30m. The day-of-week field follows UTC dates, not IST — UTC Mon–Fri at 18:45 = IST Tue–Sat at 00:15. `timedatectl` should be checked once at setup; do not assume the VPS local time.

### 0.5 Stale `sync_runs` reconciliation SQL (write into runbook now)

If a `RUNNING` row exists with no live container:

```sql
UPDATE sync_runs
SET status = 'FAILED',
    finished_at = NOW(),
    error_message = 'reconciled: no active container'
WHERE id = '<uuid>' AND status = 'RUNNING';
```

Run only after `docker ps | grep sync-staging` returns empty. Never run while a container is live. This same SQL is documented in `docs/one-vps-deployment.md` under "Stale RUNNING row reconciliation".

---

## Phase 1 — Lock + log infrastructure (VPS, one-time)

### 1.1 Create lock and log directories

```bash
sudo mkdir -p /opt/algoedgefno/locks /opt/algoedgefno/logs
sudo touch /opt/algoedgefno/locks/sync-staging.lock
sudo touch /opt/algoedgefno/locks/sync-prod.lock
sudo chown -R $(whoami):$(whoami) /opt/algoedgefno/locks /opt/algoedgefno/logs
```

### 1.2 Install logrotate config

File: `/etc/logrotate.d/algoedgefno-sync`

```text
/opt/algoedgefno/logs/*-cron.log
/opt/algoedgefno/logs/*-backfill-*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
    maxage 60
}
```

> The first glob, `*-cron.log` (not `sync-*-cron.log`), covers `notify-staging-cron.log` and any future `notify-prod-cron.log` without further edits.
>
> The second glob, `*-backfill-*.log`, covers one-shot backfill logs (e.g. `staging-backfill-2024-01-15-to-2026-05-13.log`) that would otherwise sit unrotated outside the `*-cron.log` glob and accumulate forever.
>
> `maxage 60` is required because backfill logs are not re-appended after the run completes: with `notifempty`, the first rotation moves the content into `.1` and truncates the original; subsequent rotations skip the now-empty file, so `.1` would never age through the `rotate 30` chain. `maxage 60` deletes any rotated file older than 60 days regardless of rotation count. Safe to apply to the cron logs too — they rotate daily and naturally cycle through the chain well before 60 days.

Validate: `sudo logrotate -d /etc/logrotate.d/algoedgefno-sync` (dry run, no errors).

**How logrotate behaves here:**

- Rotates **once per scheduled invocation** (Ubuntu's `cron.daily`), not once per log entry.
- Everything accumulated in the current log since the last rotation is moved into `.1` as one chunk.
- `copytruncate` copies the file then truncates the original to zero so a running process keeps writing to the same fd.
- `delaycompress` means `.1` stays uncompressed; `.2` onwards are compressed.

---

## Phase 2 — Wrap manual command in flock (runbook change)

The documented manual staging-sync command in `docs/one-vps-deployment.md` is already wrapped in `flock` against the same lock file cron uses:

```bash
flock -n /opt/algoedgefno/locks/sync-staging.lock -c \
  'cd /opt/algoedgefno/compose && docker compose --profile sync-staging run --rm sync-staging'
```

This makes the cron-vs-manual race impossible.

---

## Phase 3 — Enable cron for staging only

### 3.1 Crontab entry (root crontab on the VPS)

```text
45 18 * * 1-5 flock -n /opt/algoedgefno/locks/sync-staging.lock -c 'cd /opt/algoedgefno/compose && docker compose --profile sync-staging run --rm sync-staging' >> /opt/algoedgefno/logs/sync-staging-cron.log 2>&1
```

`1-5` = UTC Mon–Fri (= IST Tue–Sat at 00:15). NSE has no bhavcopy on weekends, so weekend syncs would 404 and pollute `sync_runs` with non-incident FAILED rows.

### 3.2 Verify timezone

- Confirm `timedatectl` shows the expected timezone before installing the crontab.
- On the current VPS, system time is UTC — the cron entries above are in UTC.
- Do not rely on assumptions about VPS local time.

### 3.3 Do not add a `sync-prod` cron line yet.

---

## Phase 4 — First-run verification (staging)

After the first 18:45 UTC (00:15 IST next day) firing:

- `tail -200 /opt/algoedgefno/logs/sync-staging-cron.log` — startup, catch-up behavior, clean exit, no secrets.
- `docker ps -a --filter name=sync-staging` — container exited `0`.
- SQL: `SELECT id, status, started_at, finished_at FROM sync_runs ORDER BY started_at DESC LIMIT 3;` — newest row is terminal (`COMPLETED` or `FAILED`), never stuck `RUNNING`.
- Candle coverage query — advanced if a new trading day is available, unchanged otherwise.
- If NSE data is unavailable at that hour, the job fails cleanly and `sync_runs` records `FAILED` with a sensible error message.

---

## Phase 5 — Lightweight failure alerting (before declaring stability)

A second cron at **06:00 IST = 00:30 UTC** runs `scripts/notify-staging-sync.sh`, which queries `sync_runs` for the last 12 hours and sends a Telegram message on success, failure, no-run, or check-failure.

Crontab line (root):

```text
30 0 * * 2-6 /opt/algoedgefno/scripts/notify-staging-sync.sh >> /opt/algoedgefno/logs/notify-staging-cron.log 2>&1
```

`2-6` = UTC Tue–Sat — the day after each weekday sync. UTC Mon sync (= IST Tue 00:15) is reported by the UTC Tue alert (= IST Tue 06:00); UTC Fri sync (= IST Sat 00:15) is reported by the UTC Sat alert (= IST Sat 06:00).

Token + chat ID live in `/opt/algoedgefno/env/telegram.env` (root-owned, mode `600`). The script writes the bot token to a temp curl config file so it does not appear in the process list.

> **Earlier drafts** used `30 2 * * *` (08:00 IST) and an email-based alert. Both have been superseded: Telegram replaced email (PR #50), the time moved to 06:00 IST so the operator sees the result before the workday starts, and the day-of-week field was tightened to `2-6` once a full Sat→Mon natural-failure cycle proved the failure-alert path end-to-end.

Without this cron, "staging stable" cannot be observed.

---

## Phase 6 — Production gating criteria

Do not enable `sync-prod` cron until **all** of these are true:

1. ≥5 consecutive weekday scheduled staging runs have completed with terminal state and no manual intervention.
2. At least one of those runs occurred on a Monday (covers weekend gap behavior).
3. At least one failure has been observed and recovered cleanly (either induced or natural), or 10 consecutive successes if no failure has occurred.
4. Log rotation has run at least once and produced a rotated `.1` file.
5. The failure-alert job in Phase 5 has fired correctly at least once in a controlled test (e.g. temporarily flip the alert to look for a non-existent status to force a `NO RUN` or failure path).

---

## Phase 7 — Enable production sync

When Phase 6 is satisfied:

- `prod.env` has `APP_ENV=production`, prod DB credentials, `SYNC_ENABLED=true`.
- Crontab adds:

  ```text
  15 19 * * 1-5 flock -n /opt/algoedgefno/locks/sync-prod.lock -c 'cd /opt/algoedgefno/compose && docker compose --profile sync-prod run --rm sync-prod' >> /opt/algoedgefno/logs/sync-prod-cron.log 2>&1
  ```

  And a matching prod alert at UTC Tue–Sat 00:30:

  ```text
  30 0 * * 2-6 /opt/algoedgefno/scripts/notify-prod-sync.sh >> /opt/algoedgefno/logs/notify-prod-cron.log 2>&1
  ```

- Manual prod-sync command is also wrapped in `flock` against `/opt/algoedgefno/locks/sync-prod.lock`.
- Separate alert cron mirrors the staging alert, pointed at prod DSN (new `scripts/notify-prod-sync.sh` or parameterized variant).
- Run Phase 4 verification again, this time against production.

---

## Out-of-scope (explicitly deferred)

- Built-in concurrency lock in Go code.
- A formal scheduler wrapper / sync orchestrator service.
- Multi-host scheduling.

These can be reconsidered after production has run cleanly for a month.

---

## Operating rules (permanent)

1. Every manual and scheduled sync invocation goes through `flock` against the per-env lock file. No bare `docker compose run`.
2. `sync_runs` rows are never edited manually except via the reconciliation SQL in 0.5, and only after `docker ps` confirms no live container.
3. `SYNC_ENABLED=false` is the kill switch; flip it before investigating any incident that might involve bad upstream data.
4. Staging and production cron windows never overlap. Any future schedule change preserves a ≥30-minute gap.
5. Log files are append-only and rotated by logrotate; never `rm` them manually.

---

## Debugging "the cron didn't fire"

**First diagnostic, always:** `date` + `crontab -l`. Compare current server time against the scheduled time. The job may simply not have fired yet today. Only proceed to syslog/journalctl/log-file checks if the scheduled time has clearly passed.

After that:

- `journalctl -u cron --since "yesterday" | grep -i "<keyword>"` — did cron try to run it?
- `ls -la /opt/algoedgefno/logs/` — did the redirect create the file?
- `stat /var/spool/cron/crontabs/root` — when was the crontab last installed?
