# Production Checklist — algoedgefno-backend

Run through this fully before every production deploy. Do not skip items.

---

## 1. Secrets & tokens (CRITICAL — do first)

- [ ] `APP_SECRET_TOKEN` is set to a strong random value (not empty, not the example value)
- [ ] `JWT_SECRET` is set to a strong random value (not `change-this-in-production`)
- [ ] `.env` file is NOT present on the server — env vars are set directly via system environment or secrets manager
- [ ] `.env` is in `.gitignore` and has never been committed — verify with `git log --all -- .env`

**How to generate strong tokens:**
```bash
openssl rand -hex 32   # generates a secure 64-character hex token
```
Run this twice — once for `APP_SECRET_TOKEN`, once for `JWT_SECRET`. Never reuse them.

---

## 2. Database

- [ ] TimescaleDB is running and reachable
- [ ] All migrations have been applied — run `migrate ... up` and confirm `no change`
- [ ] `DB_USER` is set and not the default `algoedge`
- [ ] `DB_PASSWORD` is set to a strong value and not the default `algoedge`
- [ ] `DB_NAME` is set and not the default `algoedgefno`
- [ ] DB is not exposed on a public port — only accessible from the app server

---

## 3. Server config

- [ ] `ENV=production` is set — this enables startup secret validation and Gin release mode
- [ ] `PORT` is set correctly
- [ ] CORS is tightened — `cors.Default()` in `main.go` allows all origins, restrict to your Android app's origin before go-live

---

## 4. Verify startup

- [ ] Start the server and confirm it starts without `log.Fatal` errors
- [ ] Hit `/health` endpoint and confirm `200 OK`
- [ ] Hit a protected endpoint without a token — confirm `401 Unauthorized`
- [ ] Hit a protected endpoint with the correct `APP_SECRET_TOKEN` — confirm it works

---

## 5. Before every deploy (ongoing)

- [ ] Run `go build ./...` — confirm no compile errors
- [ ] Run `go vet ./...` — confirm no vet warnings
- [ ] Confirm no `.env` file is being copied to the server
- [ ] If schema changed — migration files are present and tested locally first
