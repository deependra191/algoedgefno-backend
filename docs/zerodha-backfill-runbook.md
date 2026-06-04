# Zerodha Intraday Backfill Runbook — algoedgefno-backend

The **how** for getting clean 1-min intraday history into the **local** database,
so strategies can start being validated against real intraday data. This is the
implementation companion to `docs/data-sourcing-policy.md` — that doc is the
*what & why* and the rights boundary; this one is the build plan.

**Scope of this runbook:** policy-doc workstreams **1 (instrument model), 2
(Zerodha deep backfill), 4 (coverage metadata)** — i.e. *fill the local DB and
make coverage queryable*. **Out of scope** (separate later plans): workstream 3
(AngelOne ongoing top-up) and workstream 5 (the net-new intraday **fill engine**).
Strategies validate end-to-end on intraday fills only once workstream 5 lands;
this runbook stops at "trustworthy 1-min data is in the DB and queryable."

**Non-negotiable constraints (from the policy doc, restated as build rules):**

- All broker tooling and data are **local-only**. Never on the VPS, never in
  staging or production, never in a deployable image.
- The backfill tooling takes **local DB credentials only**. No VPS creds, no
  `DATABASE_URL` that could point at staging/prod. No embedded credentials.
- It is **personal R&D tooling, not product code** — CLAUDE.md rule 6
  (`MarketDataProvider`) explicitly does **not** apply. It is never registered in
  the provider registry and never "promoted" into a provider.

---

## 0. Where the code lives — the deployable-path boundary

The single most important structural decision, because it is what mechanically
keeps broker code off the VPS.

**Tooling root: `local-rnd/` at the repo root.** The production `Dockerfile`
copies only `cmd/`, `internal/`, `migrations/`, `scripts/` into the build (it
deliberately avoids `COPY . .` — see `Dockerfile:21-22,69-70`). Anything under
`local-rnd/` is therefore never in the image. Concretely, the tooling must **not**
live under `cmd/` (those binaries are built and shipped) or `scripts/` (copied to
`/app/scripts`, `Dockerfile:70`).

Three mechanical guards, all required, so "never ships" is enforced and not just
intended:

1. **Path** — `local-rnd/` is outside every `COPY` in the Dockerfile. (Primary.)
2. **`.dockerignore`** — add a `local-rnd/` line. The file already excludes
   `scratch/`, `.claude/`, `deploy/`, `docs/`; this belt-and-suspenders blocks the
   path even if a future `COPY` is broadened.
3. **`.go-arch-lint.yml`** — the config maps **every** package to an explicit
   component; an unmapped top-level package **fails CI lint**. Register a
   `local_rnd` component (mirror the `cmd_*` pattern). Granting it
   `anyProjectDeps: true` keeps it able to import `internal/storage` etc. while
   still being linted. Example to add under `components:` and `deps:`:

   ```yaml
   # components:
   local_rnd:
     in: local-rnd/...

   # deps:
   local_rnd:
     anyProjectDeps: true
     anyVendorDeps: true
   ```

**What it imports.** `local-rnd/` is inside the Go module, so it can import
`internal/` packages directly. Reuse — do not re-implement:

| Need | Reuse |
|---|---|
| DB pool from local config | `config.Load()` + `database.Connect(cfg)` |
| Insert candles idempotently | `storage.NewCandleStore(pool).InsertBatchIgnoreDuplicates` |
| Create/find instruments | `storage.NewInstrumentStore(pool).UpsertBatch` / `List` |
| Record a backfill run | `storage.NewSyncRunStore(pool)` (`Create` + `Complete`) |
| Domain types | `models.Candle`, `models.Instrument`, `models.SyncRun` |

The Kite HTTP client itself lives **in `local-rnd/`** (e.g.
`local-rnd/kite/`), not in `internal/providers/` — putting it under providers
would invite the `MarketDataProvider` requirement and registry wiring this tooling
must avoid.

---

## 1. Prerequisites — Kite Connect access

- A **Kite Connect** developer app: gives `api_key` + `api_secret`. The historical
  data add-on is a paid monthly subscription (policy doc records **₹500 for a
  single month** — *verify current pricing at signup*; take one month, pull the
  deep history, stop renewing).
- **Access token is daily.** The login flow is: open the Kite login URL with
  `api_key` → user authorises → redirect carries a `request_token` → exchange
  `request_token` + `api_secret` for an `access_token` valid until ~6am next day.
  For a multi-day backfill this is a **manual once-a-day step**; document it in the
  tool's README so the operator knows to refresh.
- Secrets live in the **local `.env`** (gitignored; hard rule 1) and are read via
  env vars — never committed, never embedded in source. Suggested keys:
  `KITE_API_KEY`, `KITE_API_SECRET`, `KITE_ACCESS_TOKEN`.

---

## 2. Phase 0 — Instrument model verification (workstream 1)

**Finding: no migration is needed for the futures/options shape.** The
`instruments` table (`migrations/000002_instruments.up.sql`) already carries
`underlying`, `expiry`, `strike`, `option_type`, `lot_size`, with
`UNIQUE(symbol, exchange)`. It can already represent a dated futures contract and
(later) an option contract. Workstream 1 is therefore a **verification**, not a
schema change.

**Futures as a roll series — v1 decision.** The codebase already models
continuous futures as a synthetic instrument: symbol `<UNDERLYING>-FUTCONT`
(`models.ContinuousFuturesSuffix`), type `FUTIDX_CONT` / `FUTSTK_CONT`, populated
today by the NSE EOD provider's near-month stitch.

> **v1 backfill targets the continuous stream**, mapped onto the existing
> `-FUTCONT` instruments, via Kite's `continuous=1` parameter (see §3).
> Per-monthly-contract rows (each `NIFTY25JUNFUT` as its own instrument) are
> **deferred** — backfilling them needs `instrument_token`s for *expired*
> contracts, which Kite's instruments dump does not list (it carries active
> contracts only). The continuous series is enough to validate intraday
> strategies; true per-contract roll modelling is a later, additive step (new
> instrument rows + new candles, no redesign).

**Micro-task — start a daily instruments-dump snapshot now (free, high-leverage).**
Kite's `/instruments` CSV lists only *currently-tradeable* contracts; once a
contract expires, its `instrument_token` is dropped and becomes undiscoverable
(there is no official lookup; the unofficial `exchange_token × 256` reconstruction
is undocumented and unreliable — do not build on it). So **today's** continuous
flag is the only route to expired-period history we don't already have a token
for. The permanent fix is forward-looking: snapshot the `/instruments` dump daily
into `local-rnd/` (a tiny CSV). Do this from day one and every future month's
per-contract token is preserved in your own archive, making true per-contract
backfill possible retroactively — without ever hitting the discovery wall again.

**Instrument rows the backfill needs (create via `UpsertBatch` if absent):**

| Instrument | exchange | instrument_type | Notes |
|---|---|---|---|
| Index spot (NIFTY 50, BANKNIFTY, FINNIFTY) | `NSE` | `INDEX` | Signal source; small universe |
| Index futures continuous | `NFO` | `FUTIDX_CONT` | `<U>-FUTCONT`, via `continuous=1` |
| Equity spot (F&O underlyings) | `NSE` | `EQ` | Start with a liquid subset |
| Equity futures continuous | `NFO` | `FUTSTK_CONT` | `<U>-FUTCONT`, via `continuous=1` |

---

## 3. Phase 1 — The Zerodha deep backfill script (workstream 2) — *the first real task*

### 3a. Kite historical API — facts and assumptions to validate live

These are read from published Kite docs; **confirm against the live API before
committing the pagination/interval constants** (policy doc's research rule):

- **Endpoint:** `GET /instruments/historical/:instrument_token/:interval` with
  `from`, `to` (IST datetimes), optional `continuous=1`, optional `oi=1`.
- **Intervals:** `minute` (= our canonical `1m`), `3minute`, `5minute`,
  `10minute`, `15minute`, `30minute`, `60minute`, `day`. **Store `minute` only**;
  coarser TFs are resampled from 1-min on demand (policy doc — 1-min is canonical),
  not stored.
- **Window cap:** minute data is capped to roughly a **60-day window per request**.
  Paginate from `2020-01-01` to today in ≤60-day windows (policy doc start year).
- **Token mapping:** the instruments dump (`GET /instruments`, CSV) maps
  `tradingsymbol` → `instrument_token`. **Active contracts only** — the reason
  expired futures/options can't be pulled per-contract, and why v1 uses
  `continuous=1` for futures.
- **Rate limit:** historical endpoint ≈ 3 req/s. Reuse the existing `-delay`
  throttle pattern from `cmd/sync/main.go` between requests.

### 3b. Canonical mapping into our schema (the details that bite)

- **Timezone — critical.** Kite candle timestamps are **IST (+05:30)**. Hard rule
  13 requires all stored timestamps be **TIMESTAMPTZ in UTC**. The script **must**
  convert IST→UTC before insert. A 1-min NIFTY bar at `09:15 IST` is stored as
  `03:45Z`. Getting this wrong silently shifts every bar by 5.5h.
- **Interval constant.** Add `CandleInterval1M = "1m"` to `internal/models/candle.go`
  alongside the existing `1d`/`5m`. (This constant addition is the *only* change
  this runbook makes inside `internal/`; it is data vocabulary, not engine logic —
  the engine work is workstream 5, out of scope.)
- **Provider tag.** Use a named constant for the vendor tag, e.g.
  `zerodha_kite`, defined once in the tooling (hard rule 17(d) — opaque external
  identifier). Every inserted `models.Candle.Provider` carries it; this is the
  **audit metadata** the policy doc describes (never the primary rights guard, but
  it makes provenance queryable).
- **Insert path.** Map each Kite OHLCV row → `models.Candle` and write via
  `InsertBatchIgnoreDuplicates`. This is **first-writer-wins** and idempotent: a
  re-run inserts 0 duplicate rows. Correcting an already-stored bad candle needs an
  explicit delete + reinsert — a plain re-run will *not* overwrite it.
- **Run record.** Wrap each instrument's backfill (or each window) in a
  `sync_runs` row: `SyncRunStore.Create` (RUNNING) → `Complete` (COMPLETED/FAILED
  + `records_processed`). Provider `zerodha_kite`; reuse `SyncTypeFull` or add a
  `backfill` sync-type constant.

### 3c. Script shape

`local-rnd/zerodha-backfill/main.go`, modelled on `cmd/sync/main.go`'s flag/loop
structure:

- **Flags:** `-from` (default `2020-01-01`), `-to` (default today), `-interval`
  (default `minute`), `-instruments` (selection: e.g. `index-spot`,
  `index-fut`, `equity-spot`, `equity-fut`, or explicit symbols), `-delay`
  (req spacing).
- **Loop:** for each selected instrument → resolve `instrument_token` → iterate
  ≤60-day windows from `-from` to `-to` → fetch → IST→UTC map → batch insert →
  record `sync_runs` → sleep `-delay`.
- **Kite client** (`local-rnd/kite/`): thin HTTP wrapper — auth header
  (`Authorization: token api_key:access_token`), historical fetch, instruments
  dump parse. No `MarketDataProvider` interface.

### 3d. Resume-on-failure & idempotency

The two existing mechanisms make resume cheap — no bespoke checkpoint store
needed:

- **Coverage is queryable from `candles` directly:** `MAX(ts)` per
  `(instrument_id, interval)` tells you where a previous run stopped; restart the
  window loop from there.
- **`InsertBatchIgnoreDuplicates` is idempotent:** overlapping a window on resume
  is harmless (0 rows re-inserted). So "resume" can simply be "re-run with the same
  `-from`" and rely on first-writer-wins, or narrow `-from` to the last covered
  date for speed.

---

## 4. Phase 2 — Coverage metadata (workstream 4)

The policy doc's fill engine (workstream 5) is **coverage-driven**: it descends to
1-min only where 1-min exists. That coverage is **not a new table** — it is a query
over `candles`:

```sql
SELECT instrument_id, interval,
       MIN(ts) AS first_bar, MAX(ts) AS last_bar, COUNT(*) AS bars
FROM candles
WHERE provider = 'zerodha_kite'
GROUP BY instrument_id, interval
ORDER BY instrument_id, interval;
```

Deliverables for this phase:

- A small **coverage report** — either the SQL above documented for ad-hoc use, or
  an optional `local-rnd/coverage/` helper that prints per-instrument 1-min
  coverage and obvious gaps (e.g. trading days with far fewer than ~375 one-min
  bars). This is the human-facing "is my data trustworthy yet?" check.
- `sync_runs` already records each backfill attempt (§3b); `ListByProvider` gives
  the run history.

This is the seam the future fill engine reads. Building the engine on top is
explicitly **out of scope here**.

---

## 5. Rights & safety guardrails (enforced during this work)

- **Local DB creds only.** The tool reads `config.Load()`; ensure the local `.env`
  points `DB_*` / `DATABASE_URL` at the **local** Postgres
  (`docker compose -f docker/docker-compose.yml up -d`), never a VPS host.
- **Never run against staging/prod.** Staging counts as production for data rights
  (shared VPS). There is no code path from this tool to a deployable DB, by design.
- **Promotion runbooks stay clear.** `docs/market-data-promotion.md` must never be
  run from this broker-data DB (policy doc — operational guards).
- **Watch backups.** Do not let any backup job copy the local broker DB → VPS.
- **No promotion.** Broker rows are never restored, synced, or promoted into a
  deployable database. The `provider = 'zerodha_kite'` tag is audit metadata, not
  the control — physical separation is.

---

## 6. Acceptance checklist (per PR)

Build & boundary:
- [ ] `go build ./...` green (incl. `local-rnd/`), `go vet ./...` clean.
- [ ] `go-arch-lint check` passes with the new `local_rnd` component.
- [ ] `local-rnd/` is in `.dockerignore`; a built image contains **no** backfill
      binary (`docker build … && docker run --rm <img> ls /app` shows none).

Data correctness (spot-check after a sample window, e.g. NIFTY spot, one week):
- [ ] 1-min rows present; count ≈ 375/trading day (09:15–15:30 IST).
- [ ] Timestamps are **UTC**: a 09:15 IST bar is stored at `…T03:45:00Z`.
- [ ] Re-running the same window inserts **0** new rows (idempotency).
- [ ] A `sync_runs` row exists per backfill, status `COMPLETED`,
      `records_processed` matching the insert count.

---

## 7. Suggested PR sequencing (CLAUDE.md rule 15 — one PR per task)

Target branch per rule 23: the data-sourcing policy doc is already merged to
`dev`, so these implementation PRs target **`dev`**; a human merges (rule 26).

1. **PR A — scaffold & boundary.** `local-rnd/` skeleton, `.dockerignore` entry,
   `.go-arch-lint.yml` `local_rnd` component, Kite client skeleton + env wiring,
   `CandleInterval1M` constant, README documenting the daily-token step. No bulk
   data pull yet (or a single-window smoke pull).
2. **PR B — index backfill.** Index spot + index-futures continuous, 2020→today;
   run it; verify coverage (§4) and the §6 checklist.
3. **PR C — equity backfill.** Extend the universe to equity spot + equity-futures
   continuous, liquid subset first (disk + token-mapping cost). Re-verify coverage.

---

## 8. Out of scope — explicit handoffs

- **Workstream 3 — AngelOne ongoing top-up.** Free recent-intraday append to keep
  the local DB current once the deep Zerodha history exists. Its own small plan;
  same `local-rnd/` home, same `InsertBatchIgnoreDuplicates` idempotency.
- **Workstream 5 — intraday fill engine (the net-new engine work).** Required
  before strategies validate intraday *end-to-end*. It needs: interval vocabulary
  beyond data (`intervalDuration` in `internal/engine/backtest.go` and
  `ValidateStrategySourceIntervals` in `internal/models/strategy_sources.go` today
  accept only `1d`/`5m` / `1d`), the lazy bar-magnifier with 1-min fill descent,
  the pessimistic-fallback rule, and the pessimistic-fallback counter. Track as a
  separate `plan-critical` effort — it changes `internal/engine`, which this
  runbook deliberately does not touch.
  - **Futures rolls & charges (parked here deliberately).** Intraday strategies
    never hold across an expiry, so no roll cost applies and the existing per-trade
    charge stack (`internal/engine/charges.go`) is already correct — continuous is
    purely a clean intraday feed. Roll cost only matters for positions held *across*
    an expiry; the roll dates are **knowable** (= monthly expiry dates, already in
    the `instruments.expiry` data / NSE calendar), so a synthetic close+reopen
    round-trip can be charged at each spanned expiry. This, plus the
    back-adjusted-vs-unadjusted handling above, is engine work — and there are
    **open owner questions on rollover** to resolve when workstream 5 is planned.

---

## 9. Open assumptions to confirm against the live Kite API before coding

- Exact minute-data window cap (the "60 days" figure) and whether it differs for
  `continuous=1`.
- How far back `continuous=1` intraday data actually reaches (does it cover 2020?).
- **Whether `continuous=1` returns back-adjusted or unadjusted prices.**
  Back-adjusted = seamless roll, no gap, but historical absolute price levels are
  distorted. Unadjusted = true prices, but a visible basis gap at each monthly
  roll, which can trigger spurious stop/target hits on the roll bar. Which one Kite
  returns determines how the engine (workstream 5) must treat roll bars — confirm
  before relying on absolute price levels in any backtest.
- `instrument_token` stability for index spot across the backfill window.
- Current historical-data add-on pricing (policy doc says ₹500/month — verify).
- Confirmed IST offset on returned candle timestamps and the exact session window
  for the per-day bar-count sanity check.
