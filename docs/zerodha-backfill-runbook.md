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
     in: local-rnd/**

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

- A **Kite Connect** developer app: gives `api_key` + `api_secret`. At signup on
  **2026-06-04**, app creation showed **500 credits**, valid for **30 days from
  creation**. Create it only when ready to run the probe/backfill window.
- **Access token is daily.** The login flow is: open the Kite login URL with
  `api_key` → user authorises → redirect carries a `request_token` → exchange
  `request_token` + `api_secret` for an `access_token` valid until ~6am next day.
  For a multi-day backfill this is a **manual once-a-day step**; document it in the
  tool's README so the operator knows to refresh.
- Secrets live in the **local `.env`** (gitignored; hard rule 1) and are read via
  env vars — never committed, never embedded in source. Suggested keys:
  `KITE_API_KEY`, `KITE_API_SECRET`, `KITE_ACCESS_TOKEN`.

---

## 1a. Phase 1a — Live probe first (read-only, the empirical gate)

**Before writing any backfill, prove what Kite actually serves.** This runbook has
twice had to mark Kite behaviour "verify against live API" (continuous mode for
expired contracts; back-adjusted vs raw prices). A small **read-only probe**
against the real account settles all of it definitively and de-risks every
downstream decision. Build this **first** and let its findings shape the rest —
*integrate and see for ourselves*, rather than plan against assumptions.

**Deliberately read-only — no DB, no inserts.** The probe only fires historical
requests and prints the raw shape. Nothing touches Postgres, so there is zero
rights exposure beyond personal-use API reads, and even the local-only DB guard
(§5) is not yet on the critical path.

Lives in `local-rnd/kite-probe/` + a tiny token helper (login `request_token` →
`access_token`). It answers, empirically:

- Does `continuous=1` + `minute` on an **expired** future return day-only / error?
  (confirm the corrected §2 claim with our own eyes)
- How far back does **index-spot 1-min** actually reach — 2020? earlier? later?
- **Active-contract** futures minute depth — how much a forward-capture buys.
- **Back-adjusted vs unadjusted** on continuous (eyeball a known monthly roll).
- Exact **timestamp format/offset**, the real **per-request window cap**, and
  rate-limit behaviour under load.

**Everything downstream is gated on these findings.** The §2 universe, §7
sequencing, and the deep-history start year are written as the *expected* shape;
the probe is licensed to overturn them. If spot 1-min only reaches 2021, or
forward-capture proves the only intraday-futures path, we **adjust the instrument
scope and the strategies we validate accordingly** rather than force the plan onto
data that isn't there.

**Live probe findings — 2026-06-04.** Run against Kite with NIFTY spot token
`256265` and active NIFTY futures token `15956226`:

- `continuous=1` + `minute` on the futures token returned HTTP 400
  `InputException`: `invalid interval for continuous data`. It does not silently
  downgrade to day candles.
- NIFTY spot 1-min (`NSE`, `NIFTY 50`) returned full 375-bar sessions for all
  sampled January dates from **2018-01-02** through **2026-01-02**. The planned
  2020 start is conservative; Kite served at least 2018 for this index.
- Timestamps are ISO-like strings with explicit IST offset, e.g.
  `2018-01-02T09:15:00+0530`; parsed offset is `+05:30`.
- The minute request cap is exactly **60 calendar days** for the sampled index:
  60 days succeeded, while 61/90/120 days returned HTTP 400 `InputException`:
  `interval exceeds max limit: 60 days`.
- A 6-request short burst against the historical endpoint returned all HTTP 200s
  in this sample. Keep throttling anyway; a small burst passing is not a licence
  to run the backfill unthrottled.
- Active NIFTY futures minute data existed for at least one sampled active-token
  date (`2026-05-05`, 375 bars). Other sampled dates returned 0 bars, so do not
  treat the probe as proof of a full active-contract life window yet.
- `continuous=1` + `day` returned daily bars around the sampled roll window;
  `continuous=0` for the same active token and old dates returned 0 bars. This
  confirms continuous daily retrieval, but does **not** by itself prove whether
  Kite's continuous prices are back-adjusted or unadjusted against a raw expired
  contract comparator.

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

> **v1 is SPOT-first — deep 1-min futures history is NOT obtainable (verified
> live on 2026-06-04).** `continuous=1` with interval `minute`, given a live NIFTY
> futures token, returned HTTP 400 `InputException` with `invalid interval for
> continuous data`. So deep **1-min** history exists only for instruments that
> **don't expire**: index spot and equity spot. The live probe showed NIFTY spot
> 1-min data at least back to **2018-01-02**; the planned `2020-01-01` start remains
> a conservative v1 boundary unless a strategy needs older regimes. For futures,
> the 1-min stream exists only for currently listed contracts; a deep 1-min
> continuous futures series is therefore impossible by backfill — it can only be
> built by **capturing active-contract minute data forward** and stitching over
> time. Per-monthly-contract backfill is doubly blocked: expired tokens are
> undiscoverable unless previously snapshotted, and `continuous=1` rejects
> non-day intervals. **Implication:** v1 fills **spot** at 1-min; futures get
> **daily** history now (via `continuous=1`) and a **forward 1-min capture**
> pipeline later. How a strategy whose fill instrument is a future gets validated
> intraday (spot-as-fill-proxy with basis, or validate on the forward-captured
> window) is a workstream-5 methodology question — flagged, not solved here.

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

**Instrument universe & what's actually obtainable (create rows via `UpsertBatch`
if absent):**

| Instrument | exchange | instrument_type | 1-min deep history? |
|---|---|---|---|
| Index spot (NIFTY 50, BANKNIFTY, FINNIFTY) | `NSE` | `INDEX` | ✅ Yes — no expiry, stable token. NIFTY spot served 1-min at least to 2018 in the live probe. Signal source. |
| Equity spot (F&O underlyings) | `NSE` | `EQ` | ✅ Yes — no expiry, stable token. Tradable; liquid subset first. |
| Index futures continuous (`<U>-FUTCONT`) | `NFO` | `FUTIDX_CONT` | ❌ No backfill — **daily** via `continuous=1`; 1-min only forward-captured. |
| Equity futures continuous (`<U>-FUTCONT`) | `NFO` | `FUTSTK_CONT` | ❌ Same as index futures. |

---

## 3. Phase 1 — The Zerodha deep backfill script (workstream 2) — *the first real task*

### 3a. Kite historical API — live-verified facts

These are now based on the 2026-06-04 live probe, with remaining uncertainties
called out in §9:

- **Endpoint:** `GET /instruments/historical/:instrument_token/:interval` with
  `from`, `to` (IST datetimes), optional `continuous=1`, optional `oi=1`.
- **Intervals:** `minute` (= our canonical `1m`), `3minute`, `5minute`,
  `10minute`, `15minute`, `30minute`, `60minute`, `day`. **Store `minute` only**;
  coarser TFs are resampled from 1-min on demand (policy doc — 1-min is canonical),
  not stored.
- **Window cap:** minute data is capped to **60 calendar days per request** in the
  live probe. A 60-day NIFTY spot request succeeded; 61/90/120 days returned
  HTTP 400 `InputException` with `interval exceeds max limit: 60 days`. Paginate
  in windows of **≤60 days**. Use `2020-01-01` as the conservative v1 start year,
  even though NIFTY spot served sampled 1-min data back to **2018-01-02**.
- **Token mapping:** the instruments dump (`GET /instruments`, CSV) maps
  `tradingsymbol` → `instrument_token`. **Active contracts only** — the reason
  expired futures/options can't be pulled per-contract.
- **Continuous mode is day-only for expired contracts (verified live).**
  `continuous=1` + `minute` returned HTTP 400 `InputException`
  (`invalid interval for continuous data`). `continuous=1` + `day` returned daily
  candles around the sampled roll window. So `continuous=1` yields deep *daily*
  futures history, never deep 1-min. This is why §2's 1-min plan is spot-only.
- **Timestamp shape:** Kite returned timestamps like
  `2018-01-02T09:15:00+0530`, i.e. IST with an explicit `+0530` offset.
- **Rate limit:** historical endpoint is expected to be ≈3 req/s. A 6-request
  short burst succeeded in the live probe, but keep the existing `-delay` throttle
  pattern from `cmd/sync/main.go` between requests; burst success is not an
  operational backfill rate.

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
  (default `minute`), `-instruments` (selection: `index-spot`, `equity-spot` →
  1-min; `index-fut`, `equity-fut` → resolve to **daily** via `continuous=1`, since
  expired-futures minute data isn't served — see §2), `-delay` (req spacing).
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
- `sync_runs` records each backfill attempt (§3b). Read run history directly from
  the table via SQL; **do not** wire the tooling to `SyncRunStore.ListByProvider` —
  it returns `entities.SyncRun`, and entities never leave the storage package
  (rule 20). Coverage over `candles` (above) is the primary trust check; `sync_runs`
  is the audit trail. (If a domain-typed list is ever needed, add a
  `[]models.SyncRun`-returning method to `SyncRunRepository` first.)

This is the seam the future fill engine reads. Building the engine on top is
explicitly **out of scope here**.

---

## 5. Rights & safety guardrails (enforced during this work)

- **Mechanical local-only guard — required in PR A, before any broker request or
  insert.** `config.Load()` + `database.Connect()` will connect to *any* host —
  including staging/prod — if the environment is wrong, so "local-only" cannot rest
  on convention. The tooling must **fail closed at startup**: refuse to run unless
  the DB target is provably local. Concretely — **reject a set `DATABASE_URL`,
  reject `APP_ENV` of `staging`/`production`, and allow only loopback / local-Docker
  DB hosts** (`localhost`, `127.0.0.1`, `::1`, the compose service host); abort with
  a clear error otherwise. This is what makes "no code path to a deployable DB" true
  by construction, not just by intent.
- **Local DB creds only.** Beyond the guard above, ensure the local `.env` points
  `DB_*` at the **local** Postgres (`docker compose -f docker/docker-compose.yml up
  -d`), never a VPS host.
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
- [ ] Local-only guard trips: setting `DATABASE_URL`, `APP_ENV=production`, or a
      non-local `DB_HOST` makes the tool abort **before** any broker call or insert.

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

0. **PR P — live probe (read-only, the empirical gate).** `local-rnd/kite-probe`
   + token helper. **No DB, no inserts.** Run it, capture the findings (§1a), and
   reconcile §2/§9 with reality **before** building the backfill. Everything below
   is provisional until this lands.
1. **PR A — scaffold & boundary.** `local-rnd/` skeleton, `.dockerignore` entry,
   `.go-arch-lint.yml` `local_rnd` component, the **mechanical local-only DB guard**
   (§5), Kite client skeleton + env wiring, `CandleInterval1M` constant, README
   documenting the daily-token step. No bulk data pull yet (the probe, PR P, already
   established the data shapes).
2. **PR B — spot 1-min backfill.** Index spot + equity spot (liquid subset first),
   2020→today at `minute`. Run it; verify coverage (§4) and the §6 checklist. This
   is the deep-1-min deliverable.
3. **PR C — futures daily + forward 1-min capture.** Index/equity futures *daily*
   history via `continuous=1` onto the `-FUTCONT` instruments, plus a forward-running
   capture of active-contract **minute** data to start building a real 1-min futures
   series over time. (Deep 1-min futures history is not backfillable — §2.)

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

## 9. Open assumptions after the live probe

- **RESOLVED:** Exact minute-data window cap is **60 calendar days** per request;
  61 days fails with `interval exceeds max limit: 60 days`.
- **RESOLVED:** `continuous=1` is **day-only**; `continuous=1` + `minute` fails
  with `invalid interval for continuous data`.
- **RESOLVED for NIFTY spot:** index spot 1-min data exists at least back to
  **2018-01-02** for `NIFTY 50` token `256265`. Keep `2020-01-01` as the v1 start
  unless a strategy specifically needs older data.
- **RESOLVED:** returned candle timestamps include the IST offset (`+0530`), and a
  normal sampled NIFTY spot session returned **375 one-minute bars** from 09:15 to
  15:29 IST.
- **RESOLVED:** signup pricing observed on 2026-06-04 was **500 credits**, valid
  for **30 days from app creation**.
- **PARTIAL:** active NIFTY futures minute data exists for the sampled active
  contract on at least one date (`2026-05-05`, 375 bars), but the probe did not
  prove minute coverage across the full listed contract life. Forward-capture
  should still run daily and coverage should be measured from stored rows.
- **OPEN:** whether `continuous=1` returns back-adjusted or unadjusted prices.
  The live probe confirmed continuous daily bars, but `continuous=0` for the same
  active token returned 0 bars on old dates, so it did not provide a raw expired
  contract comparator. Resolve this later by comparing against a preserved
  per-contract token archive, broker contract notes/reference prices, or another
  licensed/raw source before relying on absolute roll-window prices in the engine.
- **OPEN for non-NIFTY instruments:** confirm BANKNIFTY, FINNIFTY, and selected
  equity spot history depth during PR B sample runs. The NIFTY spot finding is
  strong evidence for index spot, not a blanket proof for every instrument.
