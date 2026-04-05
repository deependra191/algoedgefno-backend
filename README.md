# algoedgefno-backend

Go backend for AlgoEdgeFno — an Android-first intraday algo trading tool for Indian F&O traders. This service powers v2 features including user authentication, app config delivery, and will serve as the foundation for strategy cloud sync, historical data proxy, and WebSocket tick streaming.

## Tech stack

| Layer | Choice | Why |
|---|---|---|
| Language | Go 1.24 | Performance, simplicity, strong concurrency primitives |
| Framework | Gin | Minimal overhead, mature ecosystem; fasthttp (Fiber) has incompatibility with net/http middleware |
| Database | PostgreSQL 16 | ACID compliance required for financial data |
| ORM | GORM | Auto-migration, clean repository pattern support |
| Auth | golang-jwt/jwt v5 | Industry standard, maintained |
| Config | godotenv | Simple .env loading, 12-factor compatible |

## Local setup

**Prerequisites:** Go 1.24+, Docker with Compose

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

```
POST /api/v1/auth/register
Body: { "email": "...", "password": "...", "name": "..." }
→ { "token": "...", "user": { ... } }

POST /api/v1/auth/login
Body: { "email": "...", "password": "..." }
→ { "token": "...", "user": { ... } }
```

### App Config _(requires Authorization: Bearer <token>)_

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
