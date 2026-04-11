# Providers — algoedgefno-backend

How to add a new market data provider.

## Steps

1. Create `internal/providers/<name>/` package
2. Implement the `MarketDataProvider` interface (defined in `internal/providers/provider.go`)
3. Declare the provider's `Capability` set — be honest, only declare what it actually supports
4. Register the provider in the provider registry
5. Provider-specific types (API response structs, internal config) stay inside `internal/providers/<name>/` — never export them
6. Provider fetches raw data → normalises to `models.*` types → writes via `internal/storage/` functions
7. Services call providers through the registry, never by importing the provider package directly

## Capabilities

```go
const (
    CapEODHistory         Capability = "eod_history"          // historical daily candles
    CapIntradayActive     Capability = "intraday_active"       // intraday candles for active (non-expired) instruments
    CapIntradayExpiredFO  Capability = "intraday_expired_fo"   // intraday candles for expired F&O contracts
    CapLiveTicks          Capability = "live_ticks"            // real-time tick streaming
)
```

A provider declares only what it supports. Services check before calling:

```go
p, err := registry.GetWithCapability(CapLiveTicks)
if err != nil {
    // no live tick provider available
}
```

## Anti-lock-in rules

- Handler code never references a specific provider by name
- Services check capabilities before calling a provider — no hardcoded provider selection
- The `candles` table has a `provider` column for audit purposes, but application code never branches on its value
- Swapping a provider = new implementation + registry update. Zero handler or service changes required.

## Current providers

| Provider | Package | Capabilities |
|---|---|---|
| NSE bhavcopy | `internal/providers/nse/` | `eod_history` |
| Vendor (stub) | `internal/providers/vendor/` | `intraday_active`, `intraday_expired_fo`, `live_ticks` |

The vendor stub panics on use — it exists to reserve the interface contract until Phase 3.
