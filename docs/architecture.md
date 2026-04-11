# Architecture — algoedgefno-backend

## Package layout

```
cmd/server/main.go          — entry point, wires dependencies, starts HTTP server
internal/config/            — env config (reads .env, exposes typed Config struct)
internal/middleware/        — HTTP middleware: auth (static token + JWT), request logging
internal/models/            — shared Go structs (data types only, no DB or HTTP logic)
internal/storage/           — pgx queries, one file per table group
internal/providers/         — MarketDataProvider interface + registry
internal/providers/nse/     — NSE bhavcopy EOD provider
internal/providers/vendor/  — TrueData / Global Datafeeds stub (Phase 3)
internal/engine/            — indicators, evaluator, backtest runner (pure computation)
internal/services/          — orchestration layer (calls storage, providers, engine)
internal/handlers/          — HTTP handlers (parse request, call service, write response)
internal/routes/            — route registration (groups, middleware attachment)
migrations/                 — numbered SQL files (0001_*.up.sql / 0001_*.down.sql)
scripts/                    — one-off import scripts (e.g. Angel One historical dump)
docker/                     — docker-compose for local PostgreSQL + TimescaleDB
```

## Layer rules

- **handlers → services → storage** — never skip a layer
- Handlers never import pgx or write SQL
- Services call providers through the registry — never directly
- Storage owns all SQL; no SQL in handlers or services
- Engine is pure computation — no DB, no HTTP imports

## Dependency flow

```
handlers
  └── services
        ├── storage   (depends on models)
        ├── providers (depends on models, storage)
        └── engine    (depends on models)
```

- `models` is a shared leaf — imported by storage, engine, providers, services
- `config` is imported by main and any package that needs env values
- `middleware` is imported only by routes
- `routes` wires handlers + middleware, imported only by main
