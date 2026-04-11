package providers

import (
	"context"
)

// Capability declares what a provider can do.
type Capability string

const (
	CapEODHistory        Capability = "eod_history"
	CapIntradayActive    Capability = "intraday_active"
	CapIntradayExpiredFO Capability = "intraday_expired_fo"
	CapLiveTicks         Capability = "live_ticks"
)

// ProviderStatus is returned to the Android app via /api/v1/provider/status.
type ProviderStatus struct {
	Name         string       `json:"name"`
	Capabilities []Capability `json:"capabilities"`
	Healthy      bool         `json:"healthy"`
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
	for _, c := range p.Capabilities() {
		if c == cap {
			return true
		}
	}
	return false
}
