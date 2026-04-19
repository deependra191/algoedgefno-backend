package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
)

type MarketService struct {
	instrumentStore models.InstrumentRepository
	candleStore     models.CandleRepository
	registry        *providers.Registry
}

func NewMarketService(
	instrumentStore models.InstrumentRepository,
	candleStore models.CandleRepository,
	registry *providers.Registry,
) *MarketService {
	return &MarketService{
		instrumentStore: instrumentStore,
		candleStore:     candleStore,
		registry:        registry,
	}
}

func (s *MarketService) ListInstruments(ctx context.Context, filter models.InstrumentFilter) ([]models.Instrument, error) {
	return s.instrumentStore.List(ctx, filter)
}

func (s *MarketService) GetInstrument(ctx context.Context, id uuid.UUID) (*models.Instrument, error) {
	return s.instrumentStore.GetByID(ctx, id)
}

func (s *MarketService) GetCandles(ctx context.Context, filter models.CandleFilter) ([]models.Candle, error) {
	return s.candleStore.Query(ctx, filter)
}

func (s *MarketService) ProviderStatuses(ctx context.Context) []providers.ProviderStatus {
	return s.registry.Statuses(ctx)
}
