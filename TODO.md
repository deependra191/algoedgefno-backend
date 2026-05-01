# TODO

## Bugs

### Lot size of continuous futures instrument is non-deterministic

**File:** `internal/providers/nse/eod.go` — `SyncInstruments`

The continuous instrument (e.g. `NIFTY-FUTCONT`) is created during `SyncInstruments` by
picking the first futures row seen for each underlying. The NSE bhavcopy CSV contains all
three active contracts (near, mid, far month) and does not guarantee order by expiry. If NSE
has changed the lot size between contract months (e.g. NIFTY went from 50 → 25), whichever
contract row appears first in the CSV wins, so the continuous instrument may end up with the
wrong lot size. This lot size feeds directly into the engine's P&L calculation.

Need to decide: always take the near-month contract's lot size, or sort by expiry before
picking.
