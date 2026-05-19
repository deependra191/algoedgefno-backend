# VPS Monitoring & Alerting Setup

Off-host heartbeat monitoring for the one-VPS deployment. All alerts reach the operator via Telegram. The monitor lives at Healthchecks.io — a separate hosted service — so a full VPS outage (power, network, kernel panic) will still trigger an alert when the expected heartbeat stops arriving.

**Execution model:** all VPS commands are run by the operator over SSH. The agent does not have server access. After each step the operator pastes back relevant output before moving on.

This runbook complements `docs/scheduled-sync-setup.md` (sync cron) and `docs/backup-restore.md` (manual backup steps).

---

## Overview — what is monitored

| # | Check name | What it detects |
|---|---|---|
| 1 | prod-health-uptime | Production API `/health` endpoint unreachable |
| 2 | staging-health-uptime | Staging API `/health` endpoint unreachable |
| 3 | prod-ready-uptime | Production API `/ready` endpoint unreachable (DB connectivity) |
| 4 | vps-health-meta | VPS-side subsystem failures: disk, TLS certs, memory, CPU, container restarts, backup freshness |
| 5 | prod-sync | Prod sync cron heartbeat — no ping in scheduled window |
| 6 | staging-sync | Staging sync cron heartbeat — no ping in scheduled window |
| 7 | backup-nudge | Reserved for future scheduled-backup cron (deferred — see Backup framing section) |
| 8 | prod-alert-cron | `notify-prod-sync.sh` cron heartbeat — alert job itself has stopped |
| 9 | staging-alert-cron | `notify-staging-sync.sh` cron heartbeat — alert job itself has stopped |

Alerts for checks 1–3 (active uptime) are sent by Healthchecks.io directly when the endpoint does not respond. Alerts for checks 4–9 (cron heartbeat) are sent when the expected heartbeat ping fails to arrive within the grace window.

**Off-host property:** Healthchecks.io is a third-party hosted service. The VPS dying — cron not running, container crashing, kernel panic — causes the heartbeat to stop arriving, which triggers the alert. If the monitor were on the same VPS, a full-server failure would silently kill the alerter too.

---

## Phase 0 — Healthchecks.io account setup

1. Create an account at [healthchecks.io](https://healthchecks.io). The free tier supports 20 checks — this setup uses 8 now (check 7 is reserved).
2. Create a project named **algoedgefno-prod**.
3. Under **Integrations**, add a Telegram integration:
   - Bot token: copy from `/opt/algoedgefno/env/telegram.env` → `TELEGRAM_BOT_TOKEN`
   - Chat ID: copy from `/opt/algoedgefno/env/telegram.env` → `TELEGRAM_CHAT_ID`
4. Confirm a test alert arrives in Telegram from the Healthchecks.io integration page before proceeding.

---

## Phase 1 — Create the 8 active checks in the HC dashboard

Create each check below. For **Active uptime** checks, HC pings the URL you provide on schedule and alerts if it does not return 2xx. For **Cron heartbeat** checks, HC waits for a ping from your script and alerts if none arrives.

| # | Check name | Type | Schedule | Grace | Env var / action |
|---|---|---|---|---|---|
| 1 | prod-health-uptime | Active uptime | every 5 min | 2 min | HC pings `https://api.algoedgefno.com/health` — no env var needed |
| 2 | staging-health-uptime | Active uptime | every 5 min | 2 min | HC pings `https://staging-api.algoedgefno.com/health` — no env var needed |
| 3 | prod-ready-uptime | Active uptime | every 5 min | 2 min | HC pings `https://api.algoedgefno.com/ready` — no env var needed |
| 4 | vps-health-meta | Cron heartbeat | `*/5 * * * *` | 2 min | Copy ping URL → `HC_PING_VPS_HEALTH` in `healthchecks.env` |
| 5 | prod-sync | Cron heartbeat | `15 19 * * 1-5` | 6 h | Add ping URL to crontab line via curl suffix (see Phase 3) |
| 6 | staging-sync | Cron heartbeat | `45 18 * * 1-5` | 6 h | Add ping URL to crontab line via curl suffix (see Phase 3) |
| — | backup-nudge | — | deferred | — | Do not create yet. Tracked in `docs/post-beta-checklist.md` item 1. The vps-health.sh backup-freshness check already fires a Telegram alert; this HC slot is reserved for when a scheduled backup cron exists. |
| 8 | prod-alert-cron | Cron heartbeat | `30 0 * * 2-6` | 1 h | Copy ping URL → `HC_PING_NOTIFY_PROD` in `healthchecks.env` |
| 9 | staging-alert-cron | Cron heartbeat | `30 0 * * 2-6` | 1 h | Copy ping URL → `HC_PING_NOTIFY_STAGING` in `healthchecks.env` |

**Total checks created now:** 8. Reserved for future backup cron heartbeat: 1. Net 8 of 20 free-tier slots used.

For active uptime checks (1–3), set:
- Method: GET
- Expected status: 2xx
- Interval: 5 minutes
- Grace: 2 minutes

For cron heartbeat checks (5–6), the 6-hour grace period covers the full next-trading-day window — a sync that ran but whose ping failed to arrive should not alarm until the next expected window is also missed.

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
```

Do not add `HC_PING_SYNC_*` entries here — the sync-cron heartbeats (checks 5 and 6) are handled at the crontab line level (see Phase 3).

Verify permissions after writing:

```bash
sudo stat -c '%U %G %a' /opt/algoedgefno/env/healthchecks.env
# Expected output: root root 600
```

---

## Phase 3 — Install cron entries on the VPS

Run `sudo crontab -e` (root crontab) and add or replace the entries below.

### vps-health.sh (meta-check, check 4)

```text
*/5 * * * * /opt/algoedgefno/scripts/monitoring/vps-health.sh >> /opt/algoedgefno/logs/vps-health-cron.log 2>&1
```

The script sources `healthchecks.env` and handles the HC ping internally.

### Sync cron heartbeats (checks 5 and 6)

The sync cron jobs do not run a script that knows its own HC ping URL — they are one-liner flock commands. Two options for the heartbeat ping:

**Option A (recommended) — wrapper script:**

Create `/opt/algoedgefno/scripts/monitoring/ping-sync-completion.sh` with this content:

```bash
#!/bin/bash
set -euo pipefail
# shellcheck source=/dev/null
source /opt/algoedgefno/env/healthchecks.env
# $1 is the env var name: HC_PING_SYNC_PROD or HC_PING_SYNC_STAGING.
# This script is invoked from crontab after successful sync completion.
url="${!1:-}"
[[ -z "${url}" ]] && exit 0
cfg=$(mktemp)
chmod 600 "${cfg}"
printf 'url = "%s"\n' "${url}" > "${cfg}"
curl -fsS --max-time 10 --config "${cfg}" > /dev/null 2>&1 || true
rm -f "${cfg}"
```

Add two new env vars to `healthchecks.env`:

```bash
HC_PING_SYNC_PROD=""
HC_PING_SYNC_STAGING=""
```

Crontab lines for the syncs become:

```text
15 19 * * 1-5 flock -n /opt/algoedgefno/locks/sync-prod.lock -c 'cd /opt/algoedgefno/compose && docker compose --profile sync-prod run --rm sync-prod' && /opt/algoedgefno/scripts/monitoring/ping-sync-completion.sh HC_PING_SYNC_PROD >> /opt/algoedgefno/logs/sync-prod-cron.log 2>&1

45 18 * * 1-5 flock -n /opt/algoedgefno/locks/sync-staging.lock -c 'cd /opt/algoedgefno/compose && docker compose --profile sync-staging run --rm sync-staging' && /opt/algoedgefno/scripts/monitoring/ping-sync-completion.sh HC_PING_SYNC_STAGING >> /opt/algoedgefno/logs/sync-staging-cron.log 2>&1
```

**Option B (quick) — inline curl with --config:**

The ping URL is a credential and must not appear in argv. A one-liner approach that keeps it out of `ps`:

```bash
hc_cfg=$(mktemp); chmod 600 "$hc_cfg"; printf 'url = "%s"\n' "$(grep HC_PING_SYNC_PROD /opt/algoedgefno/env/healthchecks.env | cut -d= -f2- | tr -d '"')" > "$hc_cfg"; curl -fsS --max-time 10 --config "$hc_cfg" >/dev/null 2>&1; rm -f "$hc_cfg"
```

This is harder to read and harder to test. Option A is preferred.

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

```bash
sudo systemctl stop cron
# Wait for the grace window of the vps-health-meta check (2 minutes after the
# missed */5 ping). Telegram alert from HC: "vps-health-meta is DOWN".
sudo systemctl start cron
```

This is the most important test — it verifies the off-host monitoring property. If stopping cron does not produce an HC alert, the HC check schedule or grace window is misconfigured.

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
