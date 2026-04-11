package providers

import "context"

// Registry holds all registered MarketDataProviders.
type Registry struct {
	providers map[string]MarketDataProvider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]MarketDataProvider)}
}

func (r *Registry) Register(p MarketDataProvider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (MarketDataProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) All() []MarketDataProvider {
	result := make([]MarketDataProvider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// Statuses returns ProviderStatus for all registered providers.
func (r *Registry) Statuses(ctx context.Context) []ProviderStatus {
	var statuses []ProviderStatus
	for _, p := range r.providers {
		statuses = append(statuses, ProviderStatus{
			Name:         p.Name(),
			Capabilities: p.Capabilities(),
			Healthy:      p.Healthy(ctx),
		})
	}
	return statuses
}
