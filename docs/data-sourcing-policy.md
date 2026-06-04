# Data Sourcing & Rights Policy — algoedgefno-backend

How market data is sourced, where it is allowed to live, and what may be shown
to commercial users. **Forward-looking / planning doc** — none of the broker
backfill tooling below is built yet. It records the decisions made before the
data-provider work is picked up so they are not re-litigated or violated later.

This refines two earlier records that are now stale:
`docs/decisions.md` ("Data phases", "Angel One historical import is a script")
and `docs/roadmap.md` Phase 2/3. Where they disagree, this doc wins.

---

## Context & problem

**Where we are:** the backend has only NSE EOD (daily) candles, via the `nse_eod`
bhavcopy provider. Enough for daily/weekly backtesting; no intraday data at all.

**What we need:** the strategies are intraday — signals on 15-min / hourly / daily
bars, with stops and targets that fire *inside* a bar. Validating them (and
informing the owner's own trading) needs intraday history (1-min/15-min) for
futures and spot. There is no commercial reason to pay a data vendor before the
strategies are proven.

**Two forces shape every decision below:**

1. **Rights.** Cheap intraday is available from brokers (Zerodha, AngelOne) but
   only under personal-use licences — it cannot be shown to commercial users, raw
   or derived. So broker data is for local validation only; production stays on
   licensed data and pays a vendor later, once the product is proven.
2. **Intrabar resolution.** Strategies decide on coarse bars, but stops/targets
   fire within a bar, where OHLC alone cannot say whether stop or target hit
   first. Accurate backtesting needs 1-min data to resolve fills — even though no
   signal ever runs below 15-min.

Every decision here falls out of balancing those two against a small VPS and no
desire to pay for data prematurely.

---

## Core principle: rights boundary = deployment boundary

Every candle carries the licence of the source it came from, and **that licence
travels with everything computed from it** — P&L, equity curves, indicators,
stats. So the rule is:

> **Provenance determines rights.** Production may only serve data — *or
> anything derived from it* — whose source licence permits commercial display.

**The primary control is physical separation**, not row-level filtering. Broker
data exists only in the owner's local PostgreSQL database. It is never restored,
promoted, copied, synced, backed up, or imported into staging or production.
Production rights do not depend on filtering a mixed dataset, because broker rows
never enter a deployable database in the first place. The `candles.provider`
column is **audit metadata** and at most optional defence-in-depth — never the
primary guard.

---

## Two environments

| | **Local (developer machine)** | **Production (VPS)** |
|---|---|---|
| Purpose | Strategy R&D, validation, owner's own trading | Serve the app to users |
| Allowed data | Broker personal-use data (Zerodha, AngelOne) + anything | Only data licensed for commercial redistribution/display |
| Sources | Broker APIs, NSE bhavcopy | NSE bhavcopy (within terms) → licensed vendor |
| Backtests | Run here against local DB | Only ever against licensed data |
| Stack | `docker compose -f docker/docker-compose.yml up -d` (local PG + Timescale) | Hetzner CX22 |

The local dataset is a **validation oracle**: its numbers inform the owner's
decisions only. Once a strategy is validated locally, it is **re-run in
production against licensed data**, and *those* runs produce anything a user
sees. Broker-sourced numbers never reach users — not even derived ones.

**Staging counts as production for data rights.** Staging and production share one
VPS, so broker-sourced rows are forbidden in staging exactly as in production.
"Deployable" below means staging *and* production.

---

## Data rights matrix

### NSE (the data owner's policy)

- NSE owns the data at all times; public download (bhavcopy) is **not** a
  redistribution licence. Availability ≠ rights.
- **Raw display** to external users (charts, candles, OHLC tables, quotes,
  entry/exit prices — anything reverse-engineerable back to a price) requires an
  NSE data licence, obtained via an authorised vendor.
- **Derived data** — defined by NSE as processed such that "the underlying
  market data cannot be identified, recreated or re-engineered" (e.g. strategy
  P&L, returns, Sharpe, drawdown, win-rate) — **may** be redistributed
  externally, but NSE's policy states a *separate agreement and fee* applies.
  Lower bar than raw, not a blanket free pass.
- Internal / non-display use (the owner's own backtesting) is the lightest case.

### Broker APIs — Zerodha Kite Connect, AngelOne SmartAPI (personal-use licences)

- A **contract layered on top** of the exchange data. More restrictive than NSE.
- Kite Connect terms: data "cannot be displayed to the public at large"; you may
  not "create a derivative work of … distribute, publicly display, or
  sublicense"; "Kite Connect is an order execution platform, **not a data
  distribution service**." Limited personal licence, within India, for the term
  of the agreement. Violations → access termination.
- **No derived-data lane.** NSE's derived carve-out does **not** rescue
  broker-sourced derivatives, because the binding constraint is the broker
  contract, not NSE policy. Broker data (and its derivatives) are **local /
  personal use only**.

### Authorised vendor (TrueData / Global Datafeeds — Phase 3)

- NSE-authorised redistributors. The right to show **raw** data (charts, live
  quotes) to end-users comes **bundled with the subscription**, under specified
  end-user terms. This is what the vendor fee buys beyond the data itself.
- Unlocks user-facing charts/quotes and expired-F&O options data that brokers
  structurally cannot supply.

> **Pre-launch:** confirm exact end-user display terms in writing with the
> chosen vendor's licensing team, and the NSE terms for any derived display.
> The above is read from published policy, not legal advice — get a review
> before serving real users.

---

## Sourcing plan

### Production providers (on VPS, in the registry)

| Phase | Provider | Role |
|---|---|---|
| Now | `nse_eod` (bhavcopy) | Free daily EOD candles, all instruments. Ongoing. |
| Phase 3 | Vendor (`internal/providers/vendor/`) | Live ticks, expired-F&O options, licensed user-facing display. |

### Local R&D tooling (repo-root local-only path, **never in the deployable image**, not registry providers)

| Source | Role | Cost |
|---|---|---|
| **Zerodha Kite Connect** | One-shot **deep backfill** of 1-min history (→ 2020 initially), paginated 60-day windows. Years of clean intraday. | ₹500 for a single month, then stop renewing |
| **AngelOne SmartAPI** | **Ongoing intraday top-up** of recent candles. Its shallow retention is irrelevant for keeping current. | Free |
| NSE bhavcopy | Daily EOD locally too, mirrors production source | Free |

Backfill scripts hold **local DB credentials only** — never VPS credentials.
No migration or sync ships broker rows to prod; watch backups that could copy
local → VPS.

Start the deep backfill at **2020** — enough market-regime variety (trending,
choppy, the 2020 crash, range years) that an intraday backtest is trustworthy
rather than fit to a single regime. Extend earlier later only if a strategy needs
it. The sequencing is **Zerodha first** (the deep one-time history), **AngelOne
second** (ongoing top-up once history exists), **vendor last** (Phase 3, when the
product is proven and user-facing licensed data is required).

**Code vs data — and where the code lives.** The backfill *scripts* may be
version-controlled, but **not under `scripts/`**: the production Dockerfile copies
the whole `scripts/` tree into the runtime image (`Dockerfile:70`,
`COPY scripts /app/scripts`), so anything there ships to the VPS — contradicting
"never on the VPS." Broker tooling instead lives in a dedicated repo-root
directory **outside every deployable path** (e.g. `local-rnd/`), which the
Dockerfile does not copy; add that path to `.dockerignore` as belt-and-suspenders
against a future broad `COPY`. The tooling must take its DB connection from local
config only, embed no credentials, and have no path to a staging or production
database. It is personal R&D tooling, not deployable product code — so the
`MarketDataProvider` requirement (CLAUDE.md rule 6) explicitly does **not** apply,
and it is never "promoted" into a registry provider.

*Assumption to validate at implementation:* AngelOne SmartAPI exposes a historical
candle endpoint, but its retention depth and reliability for the ongoing top-up
need confirming against the live API before committing to it. Zerodha's historical
candles, daily continuous futures, ₹500/month pricing, and display restrictions are
documented in official Kite docs/terms.

---

## Instrument scope & resolution tiering

Resolution follows the instrument you place stops/targets on (the **fill
instrument**), not the one you compute signals on. Store the finest resolution
the role needs, unless the universe is large enough to hurt.

| Instrument | Tradable? | Resolution | Status |
|---|---|---|---|
| Index spot | No — signal source | 1-min | Now. Small universe; 1-min lets you resample to any signal TF. |
| Index futures | Yes — fill | 1-min | Now. |
| Equity spot | Yes — fill | 1-min | Now (F&O underlyings; start with liquid subset if disk is tight). |
| Equity futures | Yes — fill | 1-min | Now (same scoping). |
| Index options (liquid ATM/near-OTM) | Yes — fill | 1-min | **Deferred → vendor.** |
| Equity options | Yes — fill | — | **Deferred → vendor.** |

**Why options are deferred — a data limit, not a priority call:** brokers do not
serve intraday history for *expired* option contracts (Zerodha: active contracts
only; "continuous" gives daily futures candles, no intraday options). So expired
options **cannot be backfilled at all** until the authorised vendor. The boundary
is therefore natural: options begin when vendor data exists, by which point the
product is validated and the option-strategy picture is clearer.

`instruments` schema note: confirm it can represent an option contract
(underlying, strike, expiry, CE/PE), or that adding those is a clean additive
migration — so enabling options later is data, not redesign.

**1-min is the canonical stored resolution.** 5-min and other intermediate
timeframes are resampled from 1-min on demand (SQL window functions), not stored
separately — consistent with the existing intervals decision in `decisions.md`.

**`provider` is a tag, not part of candle identity.** The primary key is
`(instrument_id, ts, interval)`. Storage inserts via `ON CONFLICT … DO NOTHING`
(`InsertBatchIgnoreDuplicates`) — **first-writer-wins**, not last. Implications:
backfill re-runs are idempotent and safe; correcting an already-stored bad candle
needs an explicit delete + reinsert, not just a re-run; and there is **no**
automatic provider override — if vendor and broker data are ever co-located (e.g.
to compare sources), a later vendor write will *not* replace an earlier broker
row, so a source-precedence plan is required before mixing.

---

## Backtest fill-resolution methodology

The hard problem: a 15-min candle gives O/H/L/C but not the **order** H and L
occurred in. If stop and target both sit inside `[Low, High]`, OHLC alone cannot
say which hit first. Naive engines resolve this optimistically and overstate
results. Approach:

- **Signal timeframe ≠ management timeframe.** Signals evaluate on closed bars of
  the strategy's TF (15-min / 1-hour / daily). No look-ahead: act on the *next*
  bar's open, never the signal bar's close.
- **Lazy bar-magnifier.** Iterate trade management at the coarse TF (≈15-min).
  Per bar, an O(1) check: do ≥2 order-sensitive levels (stop, target, same-bar
  entry) fall inside `[Low, High]`?
  - **No** → resolve at the coarse bar (the ~95% case; keeps the ~15× speed-up).
  - **Yes** → descend into the stored **1-min** candles for that window.
  - **1-min still ambiguous** (both levels in one 1-min bar) → **pessimistic
    fallback**: assume the worse outcome (stop before target). Terminal rule;
    going below 1-min is the tick rabbit hole, ruled out.
- **Gaps fill at the open**, not the level (a bar opening past the stop fills at
  the open). Add a slippage buffer on stop fills (a stop becomes a market order).
- **Coverage-driven.** The resolver descends only where 1-min coverage exists;
  otherwise it falls back to pessimistic. This makes the resolution tiering above
  work with **zero engine special-casing** — policy lives in *what was
  backfilled*, queryable as `min/max(ts)` per `(instrument, interval)` directly
  from `candles` (record each backfill run in `sync_runs`). Adding
  1-min later (e.g. options, post-vendor) is purely additive: backfill rows →
  resolver starts descending automatically.
- **Pessimistic-fallback counter.** Record, per backtest, the % of exits resolved
  by the pessimistic fallback. Low % → the result is near-truth, trust it. High %
  → that strategy needs finer data before its result is believable (the signal to
  prioritise its 1-min backfill).

Because every *in-scope* instrument is stored at 1-min, the descent always
resolves for them — pessimistic-first only ever fires on the rare residual
1-min tie. So the tradable universe gets full 1-min fill accuracy **and** the
~15× lazy-descent speed-up, with no approximation penalty. The "15-min +
pessimistic as an approximation" mode only applies to not-yet-backfilled
instruments (the deferred options).

Implementation notes:
- **Futures are a roll series**, not one instrument: store each monthly contract
  and stitch the near→next roll at expiry.
- **Signal-on-spot, fill-on-future** needs an explicit series mapping; the fill
  price is the future's, and basis means the two don't track tick-for-tick.

---

## Workstreams & sequencing

This doc is the *what* and *why*. The *how* — broker auth/token flow, rate limits,
resume-on-failure, exact script structure, schema DDL — is the **implementation
runbook** for workstreams 1, 2 and 4: see
[`docs/zerodha-backfill-runbook.md`](zerodha-backfill-runbook.md). The separable
workstreams, in dependency order:

1. **Instrument model** — confirm `instruments` can represent futures roll series
   and (later) option contracts, or that extending it is an additive migration.
2. **Zerodha deep backfill** (local script) — the one-paid-month 1-min history
   pull for index spot, index/equity futures, equity spot. **The first real task.**
3. **AngelOne ongoing top-up** (local script) — free recent-intraday append, once
   the deep history exists.
4. **Coverage metadata** — queryable from `candles`, runs recorded in `sync_runs`.
   The fill engine depends on this.
5. **Fill engine** — the lazy bar-magnifier above, coverage-driven. **Net-new
   engine work:** today the backtest runner handles a single interval per run
   (`intervalDuration` knows `1d` and `5m`; a signal-vs-`TradeCandles` seam exists
   but no 1-min fill descent). This adds dual-resolution (coarse signal bar +
   1-min fill window) and extends the interval vocabulary (`1m`, `15m`). Depends
   on data (2–3) and coverage (4) existing first.

**Sequencing reality:** production stays **daily-only for users** until the Phase
3 vendor. Intraday validated locally cannot be served to users before then —
pre-vendor it informs strategy selection and the owner's own trading only. So the
near-term goal of this work is a *trustworthy local validation pipeline*, not a
user-facing intraday feature.

---

## Operational guards

- Physical DB separation is the primary guarantee: broker rows never exist in a
  deployable (staging or production) database. `provider`-tagged filtering is at
  most optional defence-in-depth, never the control.
- **Market-data promotion runbooks** (`docs/market-data-promotion.md`) must never
  be run from a broker-data DB. They may promote only environment-neutral,
  licensed/allowed datasets — never broker-sourced rows.
- Backfill scripts: local DB connection only, no embedded credentials, no path to
  any staging or production database.
- Verify `nse_eod` production use stays within NSE's bhavcopy terms (it is served
  to users) — lean to derived outputs pre-vendor; raw display waits for the
  vendor licence.
- Storage reality (compressed, TimescaleDB): index-only is hundreds of MB; the
  F&O equity universe at 1-min is the few-GB step-up — scope to liquid names
  first. All of it is **local**, so the VPS disk is not the constraint.

---

## Pre-launch checklist (rights)

- [ ] Vendor end-user display terms confirmed in writing (raw + derived).
- [ ] NSE terms confirmed for any derived data shown to users pre-vendor.
- [ ] Legal review of the local/prod boundary and what the app displays.
- [ ] No broker-provenance rows reachable by any production code path.
