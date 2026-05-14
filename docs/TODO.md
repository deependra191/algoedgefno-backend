# TODO

## Market data parity follow-up

- Sync continuous futures candle coverage (`FUTIDX_CONT`, `FUTSTK_CONT`) into local/dev datasets so local parity matches staging for the bounded reference window.
- Day-level parity between local and staging is confirmed for `2024-01-02` through `2026-04-28`.
- The remaining candle-count delta was traced to continuous futures coverage on staging, not missing ordinary EOD candles.
