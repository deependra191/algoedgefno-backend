# Decisions — algoedgefno-backend

Architectural decisions recorded from the planning phase. Update this file when a decision changes.

---

## PostgreSQL + TimescaleDB (not SQLite)

Candle data will reach millions of rows across instruments and intervals. TimescaleDB hypertables partition by time automatically, keep time-range queries fast, and compress historical data by 90–95%. SQLite cannot scale to this volume.

## pgx/v5 over GORM

GORM abstracts away SQL in ways that are incompatible with TimescaleDB-specific DDL (hypertable creation, compression policies). Explicit SQL via pgx keeps queries readable, debuggable, and fully compatible with PostgreSQL extensions.

## Firebase Auth for v1 Android, public bootstrap app config

This is a single-owner tool, so Firebase handles identity rather than a custom login/password flow. Android obtains a Firebase ID token and exchanges it at `/api/v1/auth/session` for a backend session: a short-lived access JWT plus a rotating refresh token, both minted by the backend after it verifies the Firebase token. Tenant endpoints require the backend access JWT. `/api/v1/config/app` is public because it currently returns only static pre-login Android bootstrap config and no tenant data. JWT (golang-jwt/jwt v5) is actively used to mint backend session tokens.

## MarketDataProvider interface with Capability declarations

Provider-specific code must never leak into handlers or services. The Capability enum (eod_history, intraday_active, intraday_expired_fo, live_ticks) lets services check what a provider can do before calling it. Swapping providers requires zero handler changes.

## Numbered SQL migrations, never auto-migrate

Auto-migrate in code hides schema changes from version control and makes rollbacks impossible. Numbered SQL files in `migrations/` are explicit, reviewable, and reversible. golang-migrate/migrate handles execution.

## Single Firebase-bound owner identity in v1

v1 has a single Firebase-bound owner identity. Registration is implicit on first Firebase sign-in via `/auth/session`; no password storage, no bcrypt. Tenant data is scoped by `users.id` (the Firebase UID is the external binding). An allowlist (`ALLOWED_FIREBASE_UIDS`) gates which Firebase UIDs can sign in. Multi-user remains a future expansion of the allowlist, not a re-architecture.

## Data phases: NSE EOD → Angel One dump → vendor trial

- **Phase 1:** NSE bhavcopy gives free EOD (daily) candles for all instruments. Enough for daily/weekly strategy backtesting.
- **Phase 2:** One-off script pulls recent intraday history from Angel One API. Seeds 1-min and 15-min candles. Not a built-in provider — just a data seeding tool.
- **Phase 3:** Paid vendor (TrueData or Global Datafeeds) for live ticks and expired F&O options data depth.

## Engine runs on backend, not Android

Android OS kills background processes. Backtesting on-device would require live internet (no expired F&O data available locally). SEBI may require a static IP for data vendor connections. All computation belongs on the server.

## Docker Compose locally, Hetzner CX22 for production

Local development: Docker Compose with TimescaleDB. Production: Hetzner CX22 (~€4/month), 2 vCPU, 4 GB RAM, 40 GB SSD. With TimescaleDB compression, 5 years of 1-min candles for all F&O instruments fits comfortably under 5 GB.

## Angel One historical import is a script, not a provider

Angel One is used once (or periodically manually) to seed historical intraday data. It does not need to implement MarketDataProvider — it is a standalone script in `scripts/`. This keeps the provider registry clean.

## Candle intervals: 1-min, 15-min, 1-hour, day, week, month

5-min and other intermediate intervals can be computed on-demand from 1-min candles using SQL window functions. Storing only the above intervals avoids redundant data while covering all practical backtesting timeframes. 1-min candles are retained for accurate SL/target resolution within 15-min signal candles.

## Scope: backtesting first, watchlist during vendor trial

Feature priority matches data availability. Backtesting is possible with historical data alone. Watchlist (live quotes) requires a live tick feed, which comes in Phase 3.

## Auth and abuse-control decision

V1 uses passwordless auth without phone OTP initially. Google sign-in is the primary login method.

All costly APIs, including strategy save and backtest creation, require authentication. Backend authorization derives user identity from the verified auth token, never from request body fields.

Backtest creation is quota-limited by user_id, IP, plan, and global daily caps. Free users have strict limits on daily backtests, concurrent runs, and date-range size.

Backtests run asynchronously through a bounded worker queue so API traffic cannot directly create unbounded compute cost.

Phone OTP is deferred until there is a clear product need because SMS OTP can create avoidable cost and abuse exposure.
