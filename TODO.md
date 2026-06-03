# TODO

`TODO.md` is the source-of-truth index for work selection. Detailed execution
checklists stay in their own docs and are linked from here.

## Active queues

- [Production readiness](/Users/deependrasingh/algoedgefno-backend/docs/production-checklist.md)
  Active pre-live checklist for closed-beta launch readiness.
- [Planned post-beta hardening](/Users/deependrasingh/algoedgefno-backend/docs/post-beta-checklist.md)
  Expected follow-up hardening work after closed beta proves worth continuing.
- [Roadmap pickup](/Users/deependrasingh/algoedgefno-backend/docs/roadmap.md)
  Optional planned product and engineering work outside the hardening queues.

## Conditional backlog

These are intentionally not part of the active post-beta plan. Only pick them
up if their trigger condition happens.

### Phone OTP

Trigger:
- Real onboarding demand appears that Google/email-link cannot handle
- Abuse patterns require stronger identity binding
- Product requirements explicitly need verified phone numbers

Detail:
- `docs/post-beta-checklist.md` item previously tracked here is intentionally
  excluded from the planned post-beta queue because of cost/product tradeoffs

### GitHub-enforced merge and deploy controls

Trigger:
- A second regular contributor or operator is added
- A near miss happens around merging or deploy timing
- The repo operating model materially changes

Detail:
- `docs/post-beta-checklist.md` item previously tracked here is intentionally
  excluded from the planned post-beta queue because it is governance-triggered,
  not scheduled engineering work

## Bugs

### Lot size of continuous futures instrument is non-deterministic

**File:** `internal/providers/nse/eod.go` — `SyncInstruments`

The continuous instrument (e.g. `NIFTY-FUTCONT`) is created during
`SyncInstruments` by picking the first futures row seen for each underlying.
The NSE bhavcopy CSV contains all three active contracts (near, mid, far
month) and does not guarantee order by expiry. If NSE has changed the lot size
between contract months (e.g. NIFTY went from 50 to 25), whichever contract
row appears first in the CSV wins, so the continuous instrument may end up
with the wrong lot size. This lot size feeds directly into the engine's P&L
calculation.

Need to decide: always take the near-month contract's lot size, or sort by
expiry before picking.
