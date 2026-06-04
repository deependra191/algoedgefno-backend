# Roadmap — algoedgefno-backend

## Phase 1 — Foundation + NSE EOD (current)

- Backend skeleton: pgx, migrations, auth middleware, Docker Compose
- Schema: instruments, candles, strategies, backtest_runs, sync_runs
- NSE EOD provider: bhavcopy download → parse → daily candles
- Engine: indicators (SMA, EMA, RSI, Supertrend), evaluator, backtest runner
- API: instrument/candle queries, strategy CRUD, backtest submit/results

## Phase 2 — Broker intraday backfill (local R&D only)

- Zerodha Kite Connect: one-off deep 1-min history backfill (one paid month, paginated 60-day windows back to ~2020)
- AngelOne SmartAPI: ongoing free intraday top-up of recent candles
- Standalone scripts in a local-only repo path outside `scripts/` (the prod Dockerfile ships `scripts/` into the image) — NOT built-in providers; local dev DB only, never in the deployable image (broker data is personal-use)
- Enables 1-min/15-min intraday backtesting for futures and spot; see `docs/data-sourcing-policy.md`

## Phase 3 — Vendor trial (TrueData or Global Datafeeds)

- Evaluate: expired F&O options data depth, intraday coverage, query model
- Implement VendorProvider (replaces stub in `internal/providers/vendor/`)
- Live tick streaming: vendor → backend → Android via SSE or WebSocket
- Authorised-vendor licence unlocks raw data display to users (charts, quotes) and expired-F&O options data brokers cannot supply
- Watchlist feature goes live on Android
- Paper trading becomes possible

## Phase 4 — Android integration

- `:data:remote` module in Android: Retrofit client for backend API
- `BacktestRepository` interface in `:core:domain`
- Hilt binding swap: mock → remote
- Feature modules on Android unchanged — they use domain interfaces only
