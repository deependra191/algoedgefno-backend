# algoedgefno-backend

Go backend for AlgoEdgeFno — an Android-first intraday algo trading tool for Indian F&O traders. This service powers v2 features including user authentication, app config delivery, and will serve as the foundation for strategy cloud sync, historical data proxy, and WebSocket tick streaming.

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Language | Go 1.25 | Performance, simplicity, strong concurrency primitives |
| Framework | Gin | Minimal overhead, mature ecosystem; fasthttp (Fiber) has incompatibility with net/http middleware |
| Database | PostgreSQL 16 | ACID compliance required for financial data |
| DB access | pgx/v5 (no ORM) | Direct SQL, full PostgreSQL/TimescaleDB feature support, no ORM overhead |
| Auth (Android) | Firebase ID token → backend JWT/refresh | Identity via Firebase; backend mints short-lived JWT + rotating refresh |
| App config | Public `/config/app` | Static pre-login Android bootstrap config; no tenant data |
| Config | godotenv | Simple .env loading, 12-factor compatible |

## Local setup

**Prerequisites:** Go 1.25+, Docker with Compose

```bash
# 1. Clone and enter
git clone https://github.com/deependra191/algoedgefno-backend
cd algoedgefno-backend

# 2. Copy env template
cp .env.example .env
# Edit .env and set a strong JWT_SECRET

# 3. Start PostgreSQL
docker compose -f docker/docker-compose.yml up -d

# 4. Install dependencies
go mod tidy

# 5. Run the server
go run ./cmd/server
```

Server starts at `http://localhost:8080`.

## Build

```bash
go build -o bin/server ./cmd/server
./bin/server
```

## API reference

### Health

```
GET /health
→ { "status": "ok", "version": "0.1.0" }
```

### Auth

Android obtains a Firebase ID token, then exchanges it for a backend session
(short-lived access JWT + rotating refresh token). Tenant endpoints require the
backend access JWT.

```
POST /api/v1/auth/session
Body: { "firebaseIdToken": "<firebase-id-token>" }
→ { "accessToken": "<jwt>", "refreshToken": "<43-char-base64url>",
    "user": { "id": "<uuid>", "email": "...", "displayName": "...", "photoUrl": "..." } }

POST /api/v1/auth/refresh
Body: { "refreshToken": "<43-char-base64url>" }
→ { "accessToken": "<jwt>", "refreshToken": "<43-char-base64url>" }

POST /api/v1/auth/logout
Body: { "refreshToken": "<43-char-base64url>" }
→ 204 No Content
```

### App Config

```
GET /api/v1/config/app
→ { "navTabs": [{ "route": "...", "iconKey": "..." }, ...] }
```

## Docker Compose

```bash
# Start PostgreSQL
docker compose -f docker/docker-compose.yml up -d

# Stop
docker compose -f docker/docker-compose.yml down

# Destroy data volume
docker compose -f docker/docker-compose.yml down -v
```
