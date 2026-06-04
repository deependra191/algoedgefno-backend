# Architecture — algoedgefno-backend

## Package layout

```
cmd/server/main.go          — entry point, wires dependencies, starts HTTP server
internal/config/            — env config (reads .env, exposes typed Config struct)
internal/middleware/        — HTTP middleware: backend JWT Auth on tenant endpoints, RequestID, Logger, rate limiter, request-body limiter
internal/entities/          — DB row structs used as pgx scan targets (no JSON tags)
internal/models/            — domain objects with entity mappers (no JSON tags)
internal/storage/           — pgx queries, one file per table group (accepts/returns *models.X; entity↔model mappers private to storage)
internal/providers/         — MarketDataProvider interface + registry
internal/providers/nse/     — NSE bhavcopy EOD provider
internal/providers/vendor/  — TrueData / Global Datafeeds stub (Phase 3)
internal/engine/            — indicators, evaluator, backtest runner (pure computation)
internal/services/          — orchestration layer (calls storage, providers, engine)
internal/handlers/          — HTTP handlers (parse request, call service, write response; define local response DTOs)
internal/routes/            — route registration (groups, middleware attachment)
migrations/                 — numbered SQL files (0001_*.up.sql / 0001_*.down.sql)
scripts/                    — deployable operational scripts (deploy/smoke/notify/monitoring/security); copied into the runtime image via Dockerfile
local-rnd/                  — local-only R&D tooling (e.g. broker historical backfill); NOT copied into the image, never on the VPS — see docs/data-sourcing-policy.md
docker/                     — docker-compose for local PostgreSQL + TimescaleDB
```

## Three struct layers

Every persisted resource has three distinct struct forms, each with a single
responsibility:

| Layer | Package | Role | JSON tags |
|---|---|---|---|
| Entity | `internal/entities/` | pgx scan target, mirrors DB schema | No |
| Domain model | `internal/models/` | in-memory type for services, engine | No |
| Response DTO | `internal/handlers/*` | wire format for Android | Yes |

Entities never leave the storage layer. Storage methods accept and return
domain models (`*models.X`); entity↔model mappers are private functions
inside the relevant storage file. Services and handlers never import or
handle entity types. Handlers build a local response DTO from the domain
model before calling `c.JSON`. This keeps a DB column rename from
accidentally changing the Android API contract, and makes credential leakage
(e.g. a forgotten `json:"-"` on a password hash) impossible by construction.

## Layer rules

- **handlers → services → storage** — never skip a layer
- Handlers never import pgx or write SQL
- Services call providers through the registry — never directly
- Storage owns all SQL; no SQL in handlers or services
- Engine is pure computation — no DB, no HTTP imports
- Engine types carry no `json:` tags — handlers define response DTOs for engine output

## Dependency flow

```
handlers ── (response DTOs local to each handler file)
  └── services
        ├── storage   (returns *entities.X; depends on entities)
        ├── providers (depends on entities, storage)
        └── engine    (depends on models only)

models ── depends on entities (mappers live in models)
entities ── leaf, no internal dependencies
```

- `entities` is a leaf — imported by `storage`, `models`, and providers that build DB rows
- `models` depends on `entities` for its FromEntity/ToEntity mappers
- `config` is imported by main and any package that needs env values
- `middleware` is imported only by routes
- `routes` wires handlers + middleware, imported only by main
