package providers

import (
	"context"
	"fmt"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.ProviderLookup = (*Registry)(nil)
var _ models.ProviderStatusProvider = (*Registry)(nil)

// Registry holds all registered MarketDataProviders.
type Registry struct {
	providers map[string]models.MarketDataProvider
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]models.MarketDataProvider)}
}

// Register adds a provider to the registry, keyed by its Name().
func (r *Registry) Register(p models.MarketDataProvider) {
	r.providers[p.Name()] = p
}

// Get returns the provider with the given name, if registered.
func (r *Registry) Get(name string) (models.MarketDataProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// All returns every registered provider.
func (r *Registry) All() []models.MarketDataProvider {
	result := make([]models.MarketDataProvider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// GetWithCapability returns the first provider that has the given capability.
func (r *Registry) GetWithCapability(cap models.Capability) (models.MarketDataProvider, error) {
	for _, p := range r.providers {
		if models.HasCapability(p, cap) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no provider with capability %q", cap)
}

// Statuses returns ProviderStatus for all registered providers.
func (r *Registry) Statuses(ctx context.Context) []models.ProviderStatus {
	var statuses []models.ProviderStatus
	for _, p := range r.providers {
		statuses = append(statuses, models.ProviderStatus{
			Name:         p.Name(),
			Capabilities: p.Capabilities(),
			Healthy:      p.Healthy(ctx),
		})
	}
	return statuses
}
