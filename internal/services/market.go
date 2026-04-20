package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// MarketService provides read access to instruments and candles, and reports provider health.
type MarketService struct {
	instrumentStore models.InstrumentRepository
	candleStore     models.CandleRepository
	registry        models.ProviderStatusProvider
}

func NewMarketService(
	instrumentStore models.InstrumentRepository,
	candleStore models.CandleRepository,
	registry models.ProviderStatusProvider,
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

func (s *MarketService) ProviderStatuses(ctx context.Context) []models.ProviderStatus {
	return s.registry.Statuses(ctx)
}
