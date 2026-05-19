# VPS Monitoring & Alerting Setup

Off-host heartbeat monitoring for the one-VPS deployment. All alerts reach the operator via Telegram. The monitor lives at Healthchecks.io — a separate hosted service — so a full VPS outage (power, network, kernel panic) will still trigger an alert when the expected heartbeat stops arriving.

**Execution model:** all VPS commands are run by the operator over SSH. The agent does not have server access. After each step the operator pastes back relevant output before moving on.

This runbook complements `docs/scheduled-sync-setup.md` (sync cron) and `docs/backup-restore.md` (manual backup steps).

---

## Overview — what is monitored

| # | Check name | What it detects | Status |
|---|---|---|---|
| 1 | prod-health-uptime | Production API `/health` endpoint unreachable | **Deferred** — needs external HTTP probing; revisit when first non-friend user signs up. See Phase 1 § "External HTTP probing". |
| 2 | staging-health-uptime | Staging API `/health` endpoint unreachable | **Deferred** — same as 1. |
| 3 | prod-ready-uptime | Production API `/ready` endpoint unreachable (DB connectivity) | **Deferred** — same as 1. |
| 4 | vps-health-meta | VPS-side subsystem failures: disk, TLS certs, memory, CPU, container restarts, backup freshness | Active |
| 5 | prod-sync | Prod sync cron heartbeat — no ping in scheduled window | Active |
| 6 | staging-sync | Staging sync cron heartbeat — no ping in scheduled window | Active |
| 7 | backup-nudge | Reserved for future scheduled-backup cron | **Deferred** — see Backup framing section |
| 8 | prod-alert-cron | `notify-prod-sync.sh` cron heartbeat — alert job itself has stopped | Active |
| 9 | staging-alert-cron | `notify-staging-sync.sh` cron heartbeat — alert job itself has stopped | Active |

All active checks (4, 5, 6, 8, 9) are **cron heartbeat** — Healthchecks.io alerts when an expected ping fails to arrive within the grace window. The deferred HTTP-probe checks (1–3) require an external prober and are tracked for post-beta — see Phase 1.

**Off-host property:** Healthchecks.io is a third-party hosted service. The VPS dying — cron not running, container crashing, kernel panic — causes the heartbeat to stop arriving, which triggers the alert. If the monitor were on the same VPS, a full-server failure would silently kill the alerter too.

---

## Phase 0 — Healthchecks.io account setup

1. Create an account at [healthchecks.io](https://healthchecks.io). The free tier supports 20 checks — this setup uses 5 now; checks 1–3 (HTTP probes) and 7 (backup-cron) are deferred until later triggers (see Phase 1).
2. Create a project named **algoedgefno-prod**.
3. Under **Integrations**, add a Telegram integration:
   - Bot token: copy from `/opt/algoedgefno/env/telegram.env` → `TELEGRAM_BOT_TOKEN`
   - Chat ID: copy from `/opt/algoedgefno/env/telegram.env` → `TELEGRAM_CHAT_ID`
4. Confirm a test alert arrives in Telegram from the Healthchecks.io integration page before proceeding.

---

## Phase 1 — Create the 5 active checks in the HC dashboard

Create each check below. All are **Cron heartbeat** type — HC waits for a ping from your script on schedule and alerts if none arrives. HTTP probing (checks 1–3) is deferred — see "External HTTP probing" section after the table.

**Timezone for all HC schedules below: `Asia/Kolkata` (IST).** The VPS itself runs UTC at the OS level (see `docs/scheduled-sync-setup.md` § 0.4) — this is intentional and not changing. But HC lets you pick the schedule timezone independently, and IST display matches the wall clock you read at 3am, which makes the dashboard easier to reason about. The actual VPS crontab lines in Phase 3 remain UTC; the IST expressions below are computed to fire at the same instant in time.

**Grace policy for the variable-cadence check (4):** grace equals the cron interval. One missed cycle is silently absorbed; only TWO consecutive misses fire an alert. When the cadence tightens post-beta (`*/5 * * * *`), grace tightens with it (5 min). The fixed-cadence daily checks (5, 6, 8, 9) keep grace values calibrated to operator response time, not the schedule interval — see note below the table.

| # | Check name | Type | Schedule (IST) | Grace | Env var / action |
|---|---|---|---|---|---|
| 1 | prod-health-uptime | — | deferred | — | Do not create. See "External HTTP probing" below — moves to Kuma-on-second-VPS when traffic justifies it. |
| 2 | staging-health-uptime | — | deferred | — | Same as 1. |
| 3 | prod-ready-uptime | — | deferred | — | Same as 1. |
| 4 | vps-health-meta | Cron heartbeat | `30 * * * *` | 60 min | Copy ping URL → `HC_PING_VPS_HEALTH` in `healthchecks.env` |
| 5 | prod-sync | Cron heartbeat | `45 0 * * 2-6` | 6 h | Copy ping URL → `HC_PING_SYNC_PROD` in `healthchecks.env` |
| 6 | staging-sync | Cron heartbeat | `15 0 * * 2-6` | 6 h | Copy ping URL → `HC_PING_SYNC_STAGING` in `healthchecks.env` |
| 7 | backup-nudge | — | deferred | — | Do not create. Tracked in `docs/post-beta-checklist.md` item 1. The vps-health.sh backup-freshness check already fires a Telegram alert; this HC slot is reserved for when a scheduled backup cron exists. |
| 8 | prod-alert-cron | Cron heartbeat | `0 6 * * 2-6` | 1 h | Copy ping URL → `HC_PING_NOTIFY_PROD` in `healthchecks.env` |
| 9 | staging-alert-cron | Cron heartbeat | `0 6 * * 2-6` | 1 h | Copy ping URL → `HC_PING_NOTIFY_STAGING` in `healthchecks.env` |

**Total checks created now:** 5. Reserved for future use (HTTP probes 1–3, backup cron heartbeat 7): 4. Net 5 of 20 free-tier slots used — plenty of room.

For cron heartbeat checks (5–6), the 6-hour grace period covers the full next-trading-day window — a sync that ran but whose ping failed to arrive should not alarm until the next expected window is also missed. Grace is calibrated to the operator's response window (alerter at 06:00 IST), not the 24-hour cron interval — applying grace = interval here would push detection out two days, which defeats the purpose.

For cron heartbeat checks (8–9), grace = 1 h gives the alert script (which runs at 06:00 IST) enough cushion before declaring it missed without delaying the page significantly past breakfast.

### External HTTP probing (checks 1–3) — deferred

Healthchecks.io's free tier does not include outbound HTTP probing. Checks 1–3 would have covered failure modes that VPS-internal monitoring cannot:

- Backend process running but hung (container "running", endpoint times out)
- Network routing or firewall blocking the VPS from the internet
- DNS misconfigured for `api.algoedgefno.com` / `staging-api.algoedgefno.com`
- Caddy proxying to the wrong upstream
- Backend code bug returning 5xx on `/health`

**Why deferred:** during closed beta the operator is the only user. Any of the above is discoverable within seconds of opening the Android app — the marginal alerting value is small. Setting up an external prober (UptimeRobot free tier, or a self-hosted alternative) adds operational complexity that is only justified when there are real users who would hit the failure before the operator does.

**When to add it:** when the first non-friend user signs up. Recommended path at that point is to deploy **Uptime Kuma on a second machine** (Oracle Cloud Always Free VM, home Raspberry Pi, or cheapest possible second VPS). Kuma's strengths are exactly HTTP/TCP/DNS probing, with built-in Telegram integration. The same Kuma instance can also receive push pings for the cron-heartbeat checks, optionally consolidating monitoring under one tool. Kuma was rejected for the initial setup *only* because hosting it on the same VPS it monitors defeats the off-host property — a second machine resolves that.

This decision and the trigger (first non-friend user) are tracked in `docs/post-beta-checklist.md`.

---

## Phase 2 — Create `/opt/algoedgefno/env/healthchecks.env` on the VPS

This file is NOT in Git. Create it server-side, root-owned, mode 600.

```bash
sudo touch /opt/algoedgefno/env/healthchecks.env
sudo chmod 600 /opt/algoedgefno/env/healthchecks.env
sudo chown root:root /opt/algoedgefno/env/healthchecks.env
```

Content template — fill in each value from the Healthchecks.io dashboard (Phase 1):

```bash
# Healthchecks.io ping URLs — root-owned, chmod 600, never commit.
# Each URL contains the HC UUID. Treat these as credentials.
# Obtain values from the Healthchecks.io dashboard → each check → "Ping URL".

HC_PING_VPS_HEALTH=""
HC_PING_NOTIFY_PROD=""
HC_PING_NOTIFY_STAGING=""
HC_PING_SYNC_PROD=""
HC_PING_SYNC_STAGING=""
```

Any URL left empty causes the corresponding heartbeat ping to be skipped silently. Healthchecks.io will then alert "no ping received" for that check once its grace window elapses — useful as a smoke test before all five checks exist, but every URL must be set before the closed-beta gate.

Verify permissions after writing:

```bash
sudo stat -c '%U %G %a' /opt/algoedgefno/env/healthchecks.env
# Expected output: root root 600
```

---

## Phase 2.5 — Deploy scripts to the VPS

The scripts referenced in Phase 3 (`vps-health.sh`, `ping-sync-completion.sh`, and the modified `notify-{prod,staging}-sync.sh`) live in this repo; they must be copied to `/opt/algoedgefno/scripts/` on the VPS before any cron line will work. There is no automated sync — the operator does this manually.

This phase also creates `/var/lib/algoedge-monitoring/`, which `check-containers.sh` uses to persist per-container restart counts for drift detection.

From the local checkout of the merged-to-main branch (or, before merge, the open PR branch):

```bash
# Adjust to your actual local checkout path and VPS ssh target:
REPO=/Users/deependrasingh/algoedgefno-backend
VPS=root@<your-vps-host-or-ip>

ssh $VPS "mkdir -p /opt/algoedgefno/scripts/monitoring /var/lib/algoedge-monitoring"

rsync -avz --chmod=u+rwx,go+rx,go-w \
    $REPO/scripts/monitoring/ \
    $VPS:/opt/algoedgefno/scripts/monitoring/

rsync -avz --chmod=u+rwx,go+rx,go-w \
    $REPO/scripts/notify-prod-sync.sh \
    $REPO/scripts/notify-staging-sync.sh \
    $VPS:/opt/algoedgefno/scripts/

ssh $VPS "chown -R root:root /opt/algoedgefno/scripts/monitoring /opt/algoedgefno/scripts/notify-*-sync.sh /var/lib/algoedge-monitoring"
```

Verify deployment:

```bash
ssh $VPS bash -lc '
  ls -la /opt/algoedgefno/scripts/monitoring/
  ls -la /opt/algoedgefno/scripts/notify-*-sync.sh
  for f in /opt/algoedgefno/scripts/monitoring/*.sh /opt/algoedgefno/scripts/notify-*-sync.sh; do
    bash -n "$f" && echo "OK $f" || echo "FAIL $f"
  done
'
```

All eight files in `scripts/monitoring/` should be present and mode 755. Both notify scripts should have recent mtimes and be mode 755. Every `bash -n` line should say `OK`.

Dry-run `vps-health.sh` once to confirm it reaches HC:

```bash
ssh $VPS /opt/algoedgefno/scripts/monitoring/vps-health.sh
```

Expected:
- All subsystems pass → exit 0, no stderr, HC check 4 transitions to green.
- A subsystem fails → exit 1, stderr lists the failing subsystem, HC check 4 pings `/fail` and a Telegram alert lands.
- If `healthchecks.env` is missing or `HC_PING_VPS_HEALTH` is empty, you will see a clear stderr message — fix Phase 2 and re-run.

**Redeploy whenever scripts change.** Re-running the rsync block above is the standard "deploy scripts after a merge" step. It is idempotent.

---

## Phase 3 — Install cron entries on the VPS

Run `sudo crontab -e` (root crontab) and add or replace the entries below.

> **CRITICAL — replace, do not append.** The sync entries (checks 5 and 6) MODIFY the existing sync crontab lines installed per `docs/scheduled-sync-setup.md` Phase 3 and 7 — they do not coexist with the original lines. If you append the new lines without removing the old ones, both fire at the same minute: `flock -n` makes the duplicate sync non-destructive but order-dependent, and your HC sync-heartbeat ping fires only when the new line happens to win the race — a silent flakiness that produces false "no ping received" alerts.
>
> Same risk for the alert entries (checks 8 and 9): the runbook installs `30 0 * * 2-6 notify-prod-sync.sh` (and staging). If those lines were already installed per `docs/scheduled-sync-setup.md`, do not duplicate — the existing lines now ping HC automatically because the scripts source `healthchecks.env`. Just confirm the existing lines are present.
>
> Before editing: run `sudo crontab -l > /root/crontab.before.bak` so you can diff after your change (`sudo crontab -l | diff /root/crontab.before.bak -`).
>
> After editing, sanity-check there is exactly one line per logical entry:
> ```bash
> sudo crontab -l | grep -cE "sync-(prod|staging)\.lock"   # expect: 2
> sudo crontab -l | grep -c "notify-prod-sync.sh"          # expect: 1
> sudo crontab -l | grep -c "notify-staging-sync.sh"       # expect: 1
> sudo crontab -l | grep -c "vps-health.sh"                # expect: 1
> sudo crontab -l | grep -c "ping-sync-completion"         # expect: 2
> ```

### vps-health.sh (meta-check, check 4)

```text
0 * * * * /opt/algoedgefno/scripts/monitoring/vps-health.sh >> /opt/algoedgefno/logs/vps-health-cron.log 2>&1
```

Closed-beta cadence is hourly with 60-min HC grace — a single missed ping is silently tolerated; only TWO consecutive misses fire an alert. Worst-case detection lag: 120 min. Acceptable while zero external users hit the platform; trades fast signal for a low false-positive rate on a 4GB VPS that occasionally has cron hiccups.

**Grace policy:** for this check, HC grace always equals the cron interval. When the cadence tightens, grace tightens with it. Post-beta target: `*/5 * * * *` with 5-min HC grace — see `docs/post-beta-checklist.md`.

The script sources `healthchecks.env` and handles the HC ping internally. On any subsystem failure it pings `…/fail` with the diagnostic body, writes the same body to stderr (captured by `2>&1` into the cron log), and exits non-zero — exit non-zero makes manual debugging straightforward (`./vps-health.sh && echo PASS || echo FAIL`).

**MAILTO must be empty in the crontab** so cron does not email the operator on every non-zero exit. Healthchecks.io is the single alert source — a parallel cron email is duplicate noise. Confirm with:

```bash
sudo crontab -l | head -3
```

If a `MAILTO=...` line is present, replace it with `MAILTO=""` at the top of the crontab.

### Sync cron heartbeats (checks 5 and 6)

The sync cron jobs are one-liner flock commands that do not know their own HC ping URL. The committed wrapper `scripts/monitoring/ping-sync-completion.sh` looks the URL up by env-var name so the URL itself never appears in the crontab line (and therefore never in `ps`).

Crontab lines for the syncs become:

```text
15 19 * * 1-5 ( flock -n /opt/algoedgefno/locks/sync-prod.lock -c 'cd /opt/algoedgefno/compose && docker compose --profile sync-prod run --rm sync-prod' && /opt/algoedgefno/scripts/monitoring/ping-sync-completion.sh HC_PING_SYNC_PROD ) >> /opt/algoedgefno/logs/sync-prod-cron.log 2>&1

45 18 * * 1-5 ( flock -n /opt/algoedgefno/locks/sync-staging.lock -c 'cd /opt/algoedgefno/compose && docker compose --profile sync-staging run --rm sync-staging' && /opt/algoedgefno/scripts/monitoring/ping-sync-completion.sh HC_PING_SYNC_STAGING ) >> /opt/algoedgefno/logs/sync-staging-cron.log 2>&1
```

The `&&` chain ensures the HC ping only fires on successful sync. A failed sync leaves the heartbeat absent and HC alerts after the grace window.

**Why the `( ... ) >> log 2>&1` subshell grouping is required:** in POSIX shell (`/bin/sh` used by cron), `>> log 2>&1` binds to the immediately preceding command, not to the whole `&&` chain. Without the parens, `flock ... && ping-sync ... >> log 2>&1` only logs the ping wrapper's output (which is empty on success) — the sync's own output (flock messages, docker compose logs, failure diagnostics) ends up in cron's mail spool and is silently dropped when `MAILTO=""` or no mailer is installed. The subshell wraps both commands so the redirect applies to the group.

### Alert cron lines (checks 8 and 9 — already in crontab from sync setup)

These crontab lines already exist from `docs/scheduled-sync-setup.md`. They do not change — the HC ping is handled inside `notify-prod-sync.sh` and `notify-staging-sync.sh` when `HC_PING_NOTIFY_PROD` / `HC_PING_NOTIFY_STAGING` are set:

```text
30 0 * * 2-6 /opt/algoedgefno/scripts/notify-prod-sync.sh >> /opt/algoedgefno/logs/notify-prod-cron.log 2>&1
30 0 * * 2-6 /opt/algoedgefno/scripts/notify-staging-sync.sh >> /opt/algoedgefno/logs/notify-staging-cron.log 2>&1
```

### Create the monitoring log directory

```bash
sudo mkdir -p /opt/algoedgefno/logs
sudo mkdir -p /var/lib/algoedge-monitoring
```

### Update logrotate to cover vps-health-cron.log

The existing `/etc/logrotate.d/algoedgefno-sync` glob is `*-cron.log`, which already covers `vps-health-cron.log`. No change needed.

---

## Phase 4 — Verification

Run these after Phase 3 is in place. Each test confirms one subsystem check fires and produces a Telegram alert within two minutes.

### Disk test

```bash
# Fill ~8 GiB (adjust if / has less free space)
sudo fallocate -l 8G /tmp/diskfill
sudo /opt/algoedgefno/scripts/monitoring/vps-health.sh
sudo rm /tmp/diskfill
```

If `fallocate` is unavailable: `sudo dd if=/dev/zero of=/tmp/diskfill bs=1M count=8192`.
Expected: Telegram alert containing `disk:` line within 2 minutes.

### Certificate test

Temporarily inflate the threshold to force a warning (revert immediately after):

```bash
sudo sed -i 's/^readonly WARN_DAYS=14/readonly WARN_DAYS=99999/' \
    /opt/algoedgefno/scripts/monitoring/check-cert.sh
sudo /opt/algoedgefno/scripts/monitoring/vps-health.sh
sudo sed -i 's/^readonly WARN_DAYS=99999/readonly WARN_DAYS=14/' \
    /opt/algoedgefno/scripts/monitoring/check-cert.sh
```

Expected: Telegram alert containing `cert_prod:` and/or `cert_staging:` lines.

### Backup age test

```bash
sudo touch -d "8 days ago" /opt/algoedgefno/backups/dummy.dump
sudo /opt/algoedgefno/scripts/monitoring/vps-health.sh
sudo rm /opt/algoedgefno/backups/dummy.dump
```

Expected: Telegram alert containing `backup_age_7d_exceeded:` line.

### Container restart test

```bash
docker restart algoedgefno-backend-staging
# Wait for next 5-minute vps-health.sh run, or trigger it manually:
sudo /opt/algoedgefno/scripts/monitoring/vps-health.sh
```

Expected: Telegram alert containing `containers:` line with restart count increase.
The state file at `/var/lib/algoedge-monitoring/algoedgefno-backend-staging.restarts` is updated on each run, so subsequent runs will not re-alert on the same restart.

### Off-host test

This is the most important test — it verifies the off-host monitoring property. With the closed-beta hourly cadence the "wait for the alert" window is up to 75 minutes. To keep the test tractable, temporarily tighten the HC check schedule for this run.

1. In the HC dashboard, edit check 4 (`vps-health-meta`): change schedule to `*/5 * * * *` (in Asia/Kolkata is the same expression — minute-interval crons are timezone-independent), grace to `5 minutes`, save. Grace matches the cron interval per the grace policy.
2. On the VPS, also temporarily tighten the crontab line to `*/5` so the script keeps pinging on schedule:
   ```bash
   sudo crontab -e
   # change "0 * * * *" to "*/5 * * * *" on the vps-health.sh line, save
   ```
3. Wait at least 5 minutes for HC to register a fresh ping.
4. Stop cron:
   ```bash
   sudo systemctl stop cron
   ```
5. Within ~10 minutes (5 min schedule + 5 min grace) the Telegram group should receive an HC alert: "`vps-health-meta` is DOWN".
6. Restart cron and revert the temporary tightening:
   ```bash
   sudo systemctl start cron
   sudo crontab -e
   # revert "*/5 * * * *" back to "0 * * * *" on the vps-health.sh line
   ```
   In the HC dashboard, revert check 4's schedule to `30 * * * *` (IST) and grace to `60 minutes`.

If no alert arrives in step 5, the HC check schedule or grace window is misconfigured, or the Telegram integration is not enabled on this specific check.

---

## Phase 5 — Operating rules (permanent)

1. `healthchecks.env` is root-owned, mode 600, server-side only — same handling as `telegram.env`. Never copy it into Git, logs, or issue comments.
2. HC ping URLs contain the check UUID. Treat them as credentials. Do not log them, do not paste them in chat.
3. When adding a new cron script: add a corresponding HC cron heartbeat check, assign a ping URL env var in `healthchecks.env`, and source `_hc-ping.sh` in the new script.
4. When a check fires: acknowledge it in HC after investigating to stop repeat alerts during the investigation window.
5. Active uptime checks (1–3) are fire-and-forget — HC handles the polling. No maintenance needed unless the endpoint path changes.
6. `check-containers.sh` persists per-container restart counts in `/var/lib/algoedge-monitoring/`. Do not delete this directory — it is the baseline for restart-count drift detection.

---

## Phase 6 — Debugging "why didn't I get an alert?"

**1. Check the HC dashboard first.** Go to healthchecks.io → project algoedgefno-prod. Is the check red? If yes, HC detected the failure but the alert was not delivered — investigate the Telegram integration (step 2). If the check is still green, the check itself did not detect the failure — investigate the script (steps 3–4).

**2. Check Telegram integration.** In HC → Integrations, confirm the Telegram integration is active and not paused. Send a test notification from HC.

**3. Check the cron log.** On the VPS:

```bash
tail -50 /opt/algoedgefno/logs/vps-health-cron.log
```

If the log has no recent entries, cron may not be running or the crontab entry is wrong.

**4. Check cron itself.**

```bash
date && crontab -l   # compare server time to cron schedule
sudo journalctl -u cron --since "1 hour ago"
sudo systemctl status cron
```

**First diagnostic, always:** `date` + `crontab -l`. The job may simply not have fired yet. Only proceed to syslog/journalctl if the scheduled time has clearly passed.

**5. Run the script manually.**

```bash
sudo /opt/algoedgefno/scripts/monitoring/vps-health.sh
```

Check the exit code and output. If a subsystem check fails, run that check individually:

```bash
sudo /opt/algoedgefno/scripts/monitoring/check-disk.sh
sudo /opt/algoedgefno/scripts/monitoring/check-cert.sh api.algoedgefno.com
sudo /opt/algoedgefno/scripts/monitoring/check-mem.sh
sudo /opt/algoedgefno/scripts/monitoring/check-cpu.sh
sudo /opt/algoedgefno/scripts/monitoring/check-containers.sh
sudo /opt/algoedgefno/scripts/monitoring/check-backup.sh
```

---

## Backup framing — the 7-day nudge

The backup-freshness check in `vps-health.sh` (and the reserved HC slot 7) is a **behavioural reminder, not a backup-health signal.**

There is currently no automated daily backup cron — it is deferred to `docs/post-beta-checklist.md` item 1 until the first non-friend user signs up. The 7-day window prompts the operator to either:

- (a) Run `pg_dump` manually per `docs/backup-restore.md`, or
- (b) Finally install the scheduled backup cron.

When the scheduled backup cron lands, this 7-day check will effectively never fire, but it stays as a meta-safety-net for silent cron failure — if the backup cron silently stops running for 8+ days, the vps-health alert will fire.

**Do not tighten the threshold to 26 hours until daily automated backups exist.** If you tighten it prematurely, every operator holiday longer than a day will trigger a false-positive alert.
