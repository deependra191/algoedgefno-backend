# Production Checklist — algoedgefno-backend

Run through this fully before every production deploy. Do not skip items.

---

## 1. Secrets & tokens (CRITICAL — do first)

- [ ] `APP_SECRET_TOKEN` is set to a strong random value (not empty, not the example value)
- [ ] `JWT_SECRET` is set to a strong random value (not `change-this-in-production`)
- [ ] Real env files live only under `/opt/algoedgefno/env/` and `/opt/algoedgefno/compose/.env` on the VPS
- [ ] Server env files are owned by root and mode `600`; `/opt/algoedgefno/env` is mode `700`
- [ ] `.env`, `prod.env`, `staging.env`, `postgres.env`, raw service-account JSON, and backup credentials are not committed
- [ ] `.env` has never been committed — verify with `git log --all -- .env`

**How to generate strong tokens:**
```bash
openssl rand -hex 32   # generates a secure 64-character hex token
```
Run this twice — once for `APP_SECRET_TOKEN`, once for `JWT_SECRET`. Never reuse them.

---

## 2. Database

- [ ] TimescaleDB is running and reachable
- [ ] All migrations have been applied — run `migrate ... up` and confirm `no change`
- [ ] `DB_USER` is a production-only app role and includes a production marker such as `prod` or `production`
- [ ] `DB_PASSWORD` is strong, unique to production, and stored only in the server-only production env file
- [ ] `DB_NAME` is a production-only database name and includes a production marker such as `prod` or `production`
- [ ] `environment_identity` returns `production`
- [ ] DB is not exposed on a public port — only accessible from the app server

---

## 3. Server config

- [ ] `APP_ENV=production` is set — this enables startup secret validation and Gin release mode
- [ ] `GIN_MODE=release` is set
- [ ] `PORT` is set correctly
- [ ] `MIGRATIONS_PATH=file:///app/migrations`
- [ ] `AUTO_MIGRATE=false`
- [ ] Browser CORS is disabled for Android-only production; add explicit CORS later only if a browser/admin client exists
- [ ] `TRUSTED_PROXIES` is set to the private proxy range used by Caddy or left empty when no reverse proxy headers should be trusted
- [ ] `BACKEND_IMAGE` is an immutable commit SHA tag or digest, not `latest`

---

## 4. Backup & restore readiness

- [ ] A fresh production backup exists before any production migration
- [ ] Backup filename includes environment, DB name, UTC timestamp, and migration version
- [ ] The latest production backup has been restored into staging at least once
- [ ] Restored staging DB has `environment_identity=staging`
- [ ] Restore rehearsal result is recorded outside Git
- [ ] Backup and restore steps follow `docs/backup-restore.md`

---

## 5. Verify startup

- [ ] Start the server and confirm it starts without `log.Fatal` errors
- [ ] Hit `/health` endpoint and confirm `200 OK`
- [ ] Hit `/ready` endpoint and confirm `200 OK`
- [ ] Hit `/version` endpoint and confirm environment, commit, and migration version
- [ ] Hit a protected endpoint without a token — confirm `401 Unauthorized`
- [ ] Hit a protected endpoint with the correct `APP_SECRET_TOKEN` — confirm it works
- [ ] Confirm logs contain request IDs and do not contain bearer tokens, JWTs, DB passwords, or full DSNs
- [ ] Create and review a screen-by-screen smoke-test sheet before live. For each Android screen/state, list the expected test cases, identify missing/unimplemented cases first, then run proper smoke testing against the implemented flows.

---

## 6. Before every deploy (ongoing)

- [ ] Run `go build ./...` — confirm no compile errors
- [ ] Run `go vet ./...` — confirm no vet warnings
- [ ] Confirm no real env file is being copied into Git or image builds
- [ ] If schema changed — migration files are present and tested locally first
- [ ] If schema changed — fresh production backup exists before production migration
