package vendor

import (
	"context"
	"errors"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// Stub is a placeholder for a future data vendor (TrueData / Global Datafeeds).
// It declares capabilities it will eventually support but returns errors for all operations.
type Stub struct{}

func NewStub() *Stub { return &Stub{} }

func (s *Stub) Name() string { return "vendor" }

func (s *Stub) Capabilities() []models.Capability {
	return []models.Capability{
		models.CapIntradayActive,
		models.CapIntradayExpiredFO,
		models.CapLiveTicks,
	}
}

func (s *Stub) Healthy(_ context.Context) bool { return false }

func (s *Stub) SyncInstruments(_ context.Context) (int, error) {
	return 0, errors.New("vendor provider not implemented")
}

func (s *Stub) SyncCandles(_ context.Context) (int, error) {
	return 0, errors.New("vendor provider not implemented")
}
