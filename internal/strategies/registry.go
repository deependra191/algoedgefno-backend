package strategies

import "github.com/deependra191/algoedgefno-backend/internal/models"

var _ models.BuiltinStrategyLookup = (*Registry)(nil)

// Registry holds all code-defined built-in strategies.
type Registry struct {
	strategies map[string]*models.BuiltinStrategy
	order      []string
}

// NewRegistry creates a Registry pre-loaded with all built-in strategies.
func NewRegistry() *Registry {
	r := &Registry{
		strategies: make(map[string]*models.BuiltinStrategy),
	}
	r.register(MACrossover())
	return r
}

func (r *Registry) register(s *models.BuiltinStrategy) {
	r.strategies[s.ID] = s
	r.order = append(r.order, s.ID)
}

// Get returns the built-in strategy with the given slug, if it exists.
func (r *Registry) Get(slug string) (*models.BuiltinStrategy, bool) {
	s, ok := r.strategies[slug]
	return s, ok
}

// All returns all built-in strategies in registration order.
func (r *Registry) All() []*models.BuiltinStrategy {
	result := make([]*models.BuiltinStrategy, 0, len(r.order))
	for _, slug := range r.order {
		result = append(result, r.strategies[slug])
	}
	return result
}
