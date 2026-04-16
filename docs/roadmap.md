# Roadmap — algoedgefno-backend

## Phase 1 — Foundation + NSE EOD (current)

- Backend skeleton: pgx, migrations, auth middleware, Docker Compose
- Schema: instruments, candles, strategies, backtest_runs, sync_runs
- NSE EOD provider: bhavcopy download → parse → daily candles
- Engine: indicators (SMA, EMA, RSI, Supertrend), evaluator, backtest runner
- API: instrument/candle queries, strategy CRUD, backtest submit/results

## Phase 2 — Angel One historical dump

- One-off script: pull recent intraday history from Angel One API, insert into candles
- NOT a built-in provider — just a data seeding tool in `scripts/`
- Enables intraday (1-min, 15-min) backtesting on recent data

## Phase 3 — Vendor trial (TrueData or Global Datafeeds)

- Evaluate: expired F&O options data depth, intraday coverage, query model
- Implement VendorProvider (replaces stub in `internal/providers/vendor/`)
- Live tick streaming: vendor → backend → Android via SSE or WebSocket
- Watchlist feature goes live on Android
- Paper trading becomes possible

## Phase 4 — Android integration

- `:data:remote` module in Android: Retrofit client for backend API
- `BacktestRepository` interface in `:core:domain`
- Hilt binding swap: mock → remote
- Feature modules on Android unchanged — they use domain interfaces only
