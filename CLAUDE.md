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
18. **Security review before every PR** — before raising a PR, review all changed files for security issues: hardcoded secrets or default credentials reaching production, auth bypasses, SQL injection, token leakage in logs, missing input validation at system boundaries. Scan the entire file and related files for the same class of issue — never fix one instance in isolation. See `docs/production-checklist.md`.

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
