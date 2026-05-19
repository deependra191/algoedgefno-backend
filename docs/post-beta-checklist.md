# Post-beta checklist

Items consciously deferred until after the first closed-beta users exist. Created 2026-05-19. Pull from here once user feedback proves the product is worth continued hardening.

Each item has:
- **Risk being accepted** during the deferral window
- **Trigger** that should pull it forward
- **Scope sketch** for when it lands

---

## 1. Automated daily database backup

**Deferred decision:** scheduled backups not implemented for closed beta launch. Pre-migration backups (operator-discipline, per `scratch/prod-deploy-runbook.md` Phase 3) and deploy-time env-file backups (`deploy/scripts/algoedgefno-deploy-prod.sh:201`) are the only safety nets currently.

**Risk being accepted:**
- If the VPS dies, disk corrupts, an accidental `DROP TABLE` runs, or the container hits ransomware between migrations, all user data created since the last migration is lost. On a quiet week with no schema changes, that could be days of strategies and backtests.
- For closed beta with friends-as-users, the blast radius is small (apologise, ask them to re-enter). For external paying users this would be unacceptable.

**Trigger to pull this forward:**
- First non-friend external user signs up, OR
- Any user reports having spent >1 hour entering strategies, OR
- Real money is connected to a backtest result (i.e. the platform moves from research-only)

Whichever comes first.

**Scope sketch when implemented:**

- Nightly `pg_dump` of `algoedgefno_prod` via cron, ~02:00 IST (low-load hour)
- Output: `/opt/algoedgefno/backups/prod-YYYY-MM-DD.dump` with `.meta` sidecar (timestamp + migration version + image digest), mirroring the format already used in `scratch/prod-deploy-runbook.md` Phase 3
- Retention: keep 14 most recent on the VPS; auto-delete older. A "deleted by retention" log line stays in journalctl for traceability.
- Off-site encrypted copy: push to either Hetzner Storage Box (cheap, same provider — convenience but same blast radius) OR Backblaze B2 / Wasabi (cheap, different provider — proper off-site, recommended). Encrypt with `gpg --symmetric` using a key stored only in `/opt/algoedgefno/env/`.
- Backup-failure alert: Telegram via the existing notify channel (`scripts/notify-prod-sync.sh` pattern).
- Backup-freshness check: wire into the monitoring handoff (Healthchecks.io cron monitor). Alert if no new backup file in 26h.
- Monthly restore rehearsal: already documented in `docs/backup-restore.md`. Add a calendar reminder, not a cron.

**Estimated effort:** 2–3 hours scripting + 30 min Healthchecks wiring + 30 min restore rehearsal.

**Files this would touch (forward plan):**
- New: `scripts/backup-prod.sh` (the cron driver)
- New: `scripts/backup-retention.sh` (rotation logic, separate so it's testable in isolation)
- Cron entries: follow `docs/scheduled-sync-setup.md` Phase 6 pattern
- Update `docs/backup-restore.md` to point at the scripts instead of being the canonical manual procedure
- Update `docs/production-checklist.md` §4 status once shipped
- Update `docs/monitoring-setup.md` to add the freshness check
- Off-site upload step → likely uses `rclone` to B2/Wasabi; credentials in env file

---

## Add new items here

When a future scope decision pushes something to "after users exist," add it above this line with the same shape (risk / trigger / scope sketch / effort).
