# CLAUDE.md — algoedgefno-backend

## Project context

Go REST API backend for AlgoEdgeFno, an Android-first intraday algo trading platform for Indian F&O markets.
This service handles auth, app config delivery, market data ingestion, strategy storage, and backtesting execution.
Android is a thin client — all computation happens here.

## Tech stack decisions

| Decision | Choice | Reason |
|---|---|---|
| Framework | Gin | Integrates cleanly with Go ecosystem |
| Database | PostgreSQL + TimescaleDB | ACID + time-series hypertables for candle data |
| DB driver | pgx/v5 | Direct SQL, no ORM overhead, full PostgreSQL feature support |
| Migrations | golang-migrate/migrate | Numbered SQL files, CLI + library, no auto-migrate |
| Auth (v1) | Static bearer token | Single-user personal tool, no login flow needed |
| Auth (future) | JWT (golang-jwt/jwt v5) | Kept for multi-user support |
| UUID keys | github.com/google/uuid | Avoid sequential ID enumeration |

## Hard rules

1. **Never commit `.env`** — only `.env.example` goes in version control. The `.gitignore` enforces this.
2. **`internal/` package** — nothing inside `internal/` is importable from outside this module. Keep it that way.
3. **Consistent error JSON** — all error responses use `{ "error": "message" }`. No ad-hoc error shapes.
4. **Never log tokens** — Logger middleware logs method, path, status, and latency only. Never log auth headers or tokens.
5. **Handlers → services → storage** — never skip a layer. Handlers never import pgx or write SQL. Storage owns all SQL.
6. **MarketDataProvider interface mandatory** — every new data provider must implement MarketDataProvider. Never call provider code directly from handlers or services without going through the registry.
7. **Capability declarations mandatory** — every provider declares its Capability set. Services check capabilities before calling a provider.
8. **No provider-specific types outside providers/** — types defined in `internal/providers/<name>/` never leak into handlers, services, or storage.
9. **SQL lives in storage/** — all pgx queries live in `internal/storage/`. Never inline SQL in handlers or services.
10. **Numbered SQL migrations only** — migration files live in `migrations/`. Never auto-migrate in code.
11. **`internal/engine/` is pure computation** — no DB imports, no HTTP imports. Only depends on models.
12. **Static bearer token (APP_SECRET_TOKEN) for v1 Android auth** — JWT middleware kept but not required in v1.
13. **All timestamps stored as TIMESTAMPTZ in UTC** — no naive timestamps anywhere.
14. **Use pgx/v5 for all database operations** — no ORM.
15. **One PR per task** — propose plan and wait for approval before touching code.
16. **Research & decisions rule** — before recommending a library, pattern, or architectural change: research current state, state assumptions explicitly. If guessing, say so.
17. **No magic strings for values used in multiple places or with logic depending on them** — use named constants. Env var key names defined once in `config.go` are fine inline. Anything used across files, used twice with branching logic, or acting as a sentinel value must be a named constant.
19. **Named request and response structs for non-trivial handlers** — avoid inline anonymous structs or `gin.H` maps when the request or response has multiple fields that Android maps to a data class. Define named structs (e.g. `loginRequest`, `authResponse`) in the same handler file. Single-field responses (e.g. `gin.H{"navTabs": tabs}`) are fine inline. See `internal/handlers/auth.go` as the pattern to follow.
18. **Security review before every PR — full read first, fix second** — before raising a PR: (1) read every changed file in full, (2) list ALL security issues found across all files, (3) get user confirmation, (4) fix everything in one pass, build, vet, then commit. Never fix reactively one issue at a time as they are spotted — that is whack-a-mole, not a security review. Check for: hardcoded secrets or default credentials, auth bypasses, SQL injection, token leakage in logs, missing input validation, timing attacks, race conditions on shared state. See `docs/production-checklist.md`.

## Build commands

```bash
# Run dev server
go run ./cmd/server

# Build binary
go build -o bin/server ./cmd/server

# Start local PostgreSQL + TimescaleDB
docker compose -f docker/docker-compose.yml up -d

# Run migrations
migrate -path migrations/ -database "postgres://algoedge:algoedge@localhost:5432/algoedgefno?sslmode=disable" up

# Download dependencies
go mod tidy
```
