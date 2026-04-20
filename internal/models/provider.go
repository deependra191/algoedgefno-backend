package models

import (
	"context"
	"slices"
)

// Capability declares what a provider can do.
type Capability string

const (
	CapEODHistory        Capability = "eod_history"
	CapIntradayActive    Capability = "intraday_active"
	CapIntradayExpiredFO Capability = "intraday_expired_fo"
	CapLiveTicks         Capability = "live_ticks"
)

// ProviderStatus is a service-internal representation of a provider's health
// and capabilities. Handlers must map this to a local response DTO before
// serializing to JSON — it carries no `json:` tags on purpose.
type ProviderStatus struct {
	Name         string
	Capabilities []Capability
	Healthy      bool
}

// MarketDataProvider is the interface every data source must implement.
type MarketDataProvider interface {
	// Name returns the provider identifier (e.g., "nse_eod", "truedata").
	Name() string

	// Capabilities returns what this provider can do.
	Capabilities() []Capability

	// SyncInstruments fetches the current instrument list from the provider
	// and upserts into the instruments table.
	// Returns count of instruments synced.
	SyncInstruments(ctx context.Context) (int, error)

	// SyncCandles fetches candle data and inserts into the candles table.
	// The provider decides what range to sync (e.g., latest available day for EOD).
	// Returns count of candles synced.
	SyncCandles(ctx context.Context) (int, error)

	// Healthy checks if the provider's upstream source is reachable.
	Healthy(ctx context.Context) bool
}

// HasCapability checks if a provider has a given capability.
func HasCapability(p MarketDataProvider, cap Capability) bool {
	return slices.Contains(p.Capabilities(), cap)
}

// ProviderLookup is the minimal registry contract the sync service depends on.
type ProviderLookup interface {
	Get(name string) (MarketDataProvider, bool)
}

// ProviderStatusProvider is the registry contract for reporting provider health.
type ProviderStatusProvider interface {
	Statuses(ctx context.Context) []ProviderStatus
}
