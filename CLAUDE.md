# CLAUDE.md — algoedgefno-backend

## Project context

Go REST API backend for AlgoEdgeFno, an Android-first intraday algo trading platform for Indian F&O markets. This service handles auth, app config delivery, and will grow to support strategy sync, historical data proxy, and WebSocket tick streaming.

## Tech stack decisions

| Decision | Choice | Reason |
|---|---|---|
| Framework | Gin (not Fiber) | Fiber uses fasthttp which is incompatible with standard `net/http` middleware; Gin integrates cleanly with GORM, JWT libs, and the broader Go ecosystem |
| Database | PostgreSQL | ACID compliance is non-negotiable for financial user data; no eventual-consistency trade-offs |
| Auth | JWT (golang-jwt/jwt v5) | Stateless, Android-friendly, industry standard for mobile backends |
| UUID primary keys | github.com/google/uuid | Avoids sequential ID enumeration attacks on user endpoints |
| Password hashing | bcrypt cost 12 | Balances security and latency; cost 10 is minimum, 14+ is overkill for this scale |

## Hard rules

1. **Never commit `.env`** — only `.env.example` goes in version control. The `.gitignore` enforces this.
2. **Always hash passwords with bcrypt** — cost factor 12. Never store plain text or reversible hashes. Never log passwords.
3. **All endpoints except `/health` and `/api/v1/auth/*` require JWT** — enforced via the `middleware.JWTAuth` middleware in `routes/routes.go`.
4. **Repository pattern** — handlers call services, services call repositories. Handlers never import `gorm.io` or touch the DB directly.
5. **`internal/` package** — nothing inside `internal/` is importable from outside this module. Keep it that way.
6. **Consistent error JSON** — all error responses use `{ "error": "message" }`. No ad-hoc error shapes.
7. **Never log tokens** — the `Logger` middleware logs method, path, status, and latency only.

## Build commands

```bash
# Run dev server
go run ./cmd/server

# Build binary
go build -o bin/server ./cmd/server

# Download dependencies
go mod tidy

# Start local PostgreSQL
docker compose -f docker/docker-compose.yml up -d
```

## Research & decisions rule

Before recommending a library, pattern, or architectural change: research the current state (check Go module versions, read the library docs, check open issues). State your assumptions explicitly. If you are guessing, say so.
