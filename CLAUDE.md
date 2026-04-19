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
18. **Security review before every PR — full read first, fix second** — before raising a PR: (1) read every changed file in full, (2) list ALL security issues found across all files, (3) get user confirmation, (4) fix everything in one pass, build, vet, then commit. Never fix reactively one issue at a time as they are spotted — that is whack-a-mole, not a security review. Check for: hardcoded secrets or default credentials, auth bypasses, SQL injection, token leakage in logs, missing input validation, timing attacks, race conditions on shared state. See `docs/production-checklist.md`.
19. **Named request and response structs for non-trivial handlers** — avoid inline anonymous structs or `gin.H` maps when the request or response has multiple fields that Android maps to a data class. Request structs (e.g. `loginRequest`) live in the handler file — they are tightly coupled to `ShouldBindJSON` validation. Response DTOs and their mappers (e.g. `userResponse`, `toUserResponse`) live in a companion `_response.go` file (e.g. `auth_response.go`) — they are the Android contract and deserve their own file. Single-field responses (e.g. `gin.H{"navTabs": tabs}`) are fine inline.
20. **Three struct layers for persisted resources** — every DB-backed resource has (1) a pgx scan target in `internal/entities/`, (2) a domain object in `internal/models/`, and (3) a handler-local response DTO. Entities and domain models carry NO `json:` tags — only response DTOs do. Entities are internal to `internal/storage/` — they never leave the storage package. Storage methods accept and return domain models (`*models.X`); entity↔model mappers are private functions inside the relevant storage file. Services and handlers never import or handle entity types. Handlers convert domain models to local DTOs before `c.JSON`. This prevents a DB column rename from silently changing the Android API contract and makes credential leakage (e.g. a forgotten `json:"-"`) impossible by construction.
21. **Repository and engine interfaces in `internal/models/`** — `internal/models/` is the innermost layer and must import nothing beyond stdlib and `github.com/google/uuid`. Every storage-backed resource defines a repository interface in `internal/models/` (e.g. `BacktestRepository`); storage implements it, services hold the interface type — never the concrete `*storage.X` type. Every computation component that services depend on defines its contract as an interface in `internal/models/` (e.g. `BacktestEngine`); the implementation lives in `internal/engine/`. This keeps services testable without a real database or engine, and makes implementations swappable. Handlers define response DTOs for any domain or engine output before serializing — domain types never carry `json:` tags.
22. **Documentation rule — two distinct standards for comments vs. doc comments:**
   - **Inline comments inside function bodies:** default to none. Only add when the WHY is non-obvious — a hidden constraint, a subtle invariant, a workaround for a specific bug, or ordering behaviour that would surprise a reader. Never explain WHAT the code does; well-named identifiers do that.
   - **Function/method doc comments:** required for (a) all exported functions and types, (b) any unexported function implementing a non-trivial algorithm, state machine, or multi-step process. Simple constructors (`NewX`), one-line helpers, and self-explanatory names do not need one.
   - **Interface vs. implementation:** the doc comment lives on the interface — it defines the contract (what the method guarantees, what it expects). Implementations only add a doc if the concrete algorithm has non-obvious specifics beyond the interface contract (e.g. ordering invariants, concurrency behaviour, known edge cases). Never copy-paste the interface doc onto the implementation.

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
