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

## 2. External HTTP probes

**Deferred decision:** public endpoint uptime probes are not installed for closed beta. The current active Healthchecks.io setup monitors VPS-side cron heartbeats and subsystem health only; checks 1-3 in `docs/monitoring-setup.md` remain reserved for external HTTP probing.

**Risk being accepted:**
- A failure that is visible only from outside the VPS can go undetected until the operator opens the Android app or manually curls the API. Examples: DNS drift, Caddy proxy misrouting, firewall changes, public network routing issues, backend endpoint hangs, or `/ready` returning 5xx while containers still look "running".
- For friend-only closed beta this is acceptable because the operator is close to the product. For external users, this becomes user-visible downtime without independent alerting.

**Trigger to pull this forward:**
- First non-friend external user signs up, OR
- Any incident where the API is unavailable from a client while VPS-internal checks remain green.

**Scope sketch when implemented:**

- Deploy Uptime Kuma on a second machine, not on the monitored VPS.
- Add probes for:
  - production `/health`
  - staging `/health`
  - production `/ready`
- Wire Telegram alerts through Kuma.
- Optionally migrate the existing cron heartbeat pings into Kuma push monitors later, but keep the current Healthchecks.io setup until the replacement has alerted successfully in a controlled test.
- Update `docs/monitoring-setup.md` Phase 1 and `docs/production-checklist.md` §9 when shipped.

**Estimated effort:** 1-2 hours if a second host is already available; longer if a second host must be provisioned.

---

## 3. Compose-level backend healthchecks

**Deferred decision:** backend containers currently rely on Caddy/manual smoke checks plus the VPS meta-health script. The Compose file does not yet declare backend `healthcheck` entries.

**Risk being accepted:**
- Docker can report `backend-prod` or `backend-staging` as running even when the HTTP server is wedged, slow, or internally unhealthy.
- Deploy verification still catches this through scripted smoke checks, but steady-state container health is less explicit than it could be.

**Trigger to pull this forward:**
- External HTTP probes are implemented, OR
- Any incident where a backend container is running but endpoint smoke fails, OR
- The runtime image gains a small HTTP probe tool suitable for Compose healthchecks.

**Scope sketch when implemented:**

- Add backend `healthcheck` entries in `deploy/docker-compose.yml`.
- Prefer a lightweight internal probe that does not require secrets and does not log tokens.
- Use `/health` for process liveness; consider `/ready` only if restart-on-DB-unavailable is explicitly desired.
- Update `docs/one-vps-deployment.md` smoke-check notes once Compose health is active.

**Estimated effort:** 30-60 minutes if the image already has a suitable probe binary; otherwise include image/tooling work.

---

## 4. Tighten VPS meta-health cadence

**Deferred decision:** `vps-health.sh` runs hourly during closed beta, with Healthchecks.io grace set to 60 minutes. This intentionally tolerates one missed run.

**Risk being accepted:**
- Worst-case alert latency for VPS-side subsystem failures can be close to two hours.
- This is acceptable before external users because it reduces false positives on a small VPS.

**Trigger to pull this forward:**
- First non-friend external user signs up, OR
- A production incident shows the hourly cadence is too slow.

**Scope sketch when implemented:**

- Change the root crontab entry from hourly to `*/5 * * * *`.
- Update Healthchecks.io check 4 grace to 5 minutes.
- Confirm a controlled subsystem failure produces a Telegram alert at the new cadence.
- Update `docs/monitoring-setup.md` Phase 3 and `docs/production-checklist.md` §9 if the target cadence changes.

**Estimated effort:** 15-30 minutes.

---

## 5. Deploy credential hardening

**Deferred decision:** current deployment uses narrow root-owned wrappers, a limited self-hosted runner user, digest-qualified images, and root-owned server-side env files. A full secrets-manager or short-lived deploy-credential model is not implemented.

**Risk being accepted:**
- Long-lived GHCR/package credentials and server-side deployment credentials remain operationally convenient but increase blast radius if the VPS, runner user, or Docker credentials are compromised.
- Secret rotation is manual, so stale credentials may survive longer than ideal.

**Trigger to pull this forward:**
- First non-friend external user signs up, OR
- A second operator needs deployment access, OR
- Any credential exposure, runner compromise, or token-rotation incident occurs.

**Scope sketch when implemented:**

- Inventory every deploy credential: GHCR pull credentials, GitHub runner registration/token state, Firebase service-account JSON, healthcheck/Telegram URLs, DB env files, and backup/off-site storage credentials.
- Rotate long-lived tokens and document owner, scope, creation date, and next rotation date outside Git.
- Prefer least-privilege package-read tokens for GHCR and keep them only in root-owned Docker auth on the VPS.
- Evaluate a secrets manager or encrypted deployment store for server-side credentials.
- If practical, replace long-lived deploy credentials with shorter-lived or tightly scoped alternatives.
- Update `docs/one-vps-deployment.md` and `docs/production-checklist.md` after the credential model changes.

**Estimated effort:** 2-4 hours for inventory/rotation; longer if introducing a secrets manager.

---

## 6. Post-PR2 tenant identity cleanup — DONE

**Status:** Implemented. `middleware.RequireUserIdentity()` now enforces the
tenant identity invariant centrally (present, `uuid.UUID`, non-`uuid.Nil`,
otherwise `401 {"error":"missing user identity"}`), applied to the tenant route
subgroup after `middleware.Auth(...)`. The repeated handler-local
`extractUserID(c)` checks were replaced by the `mustUserID(c)` invariant helper;
`/config/app` and the `/auth/*` routes remain outside the guard. Original
deferral context retained below for history.

**Deferred decision:** the Firebase auth rollout keeps the existing repeated
handler-local `extractUserID(c)` checks for now so PR 2 stayed focused on token
exchange, launch sequencing, and production hardening. A later cleanup PR can
introduce a route invariant for tenant identity checks.

**Risk being accepted:**
- Handler code stays slightly repetitive and the route invariant is enforced in
  each handler rather than centrally.
- The current shape is already safe because handlers still reject missing,
  malformed, or `uuid.Nil` identities, but the repeated pattern is easier to
  drift over time than a single middleware guard.

**Trigger to pull this forward:**
- After PR 2 has been stable for a while, OR
- If identity-check repetition starts to spread to more tenant handlers, OR
- If a future cleanup pass wants to simplify tenant-route invariants without
  changing session/auth semantics.

**Scope sketch when implemented:**

- Add `middleware.RequireUserIdentity()` that validates `models.UserIDKey` is
  present, is a `uuid.UUID`, and is not `uuid.Nil`; otherwise abort with
  `401 {"error":"missing user identity"}`.
- Apply it only to tenant route groups, after `middleware.Auth(...)`.
- Replace repeated handler blocks like `userID, ok := extractUserID(c)` with a
  handler-local invariant helper such as `mustUserID(c)`.
- Keep `/api/v1/config/app`, `/api/v1/auth/session`, `/api/v1/auth/refresh`,
  and `/api/v1/auth/logout` outside this invariant.
- Add tests for missing, malformed, and nil identities on tenant routes, plus
  middleware ordering coverage.

**Estimated effort:** low to medium, depending on how many tenant handlers are
refactored in the same pass.

---
## Add new items here

When a future scope decision pushes something to "after users exist," add it above this line with the same shape (risk / trigger / scope sketch / effort).
