package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

type MarketService struct {
	instrumentStore *storage.InstrumentStore
	candleStore     *storage.CandleStore
	registry        *providers.Registry
}

func NewMarketService(
	instrumentStore *storage.InstrumentStore,
	candleStore *storage.CandleStore,
	registry *providers.Registry,
) *MarketService {
	return &MarketService{
		instrumentStore: instrumentStore,
		candleStore:     candleStore,
		registry:        registry,
	}
}

func (s *MarketService) ListInstruments(ctx context.Context, filter storage.InstrumentFilter) ([]models.Instrument, error) {
	ents, err := s.instrumentStore.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]models.Instrument, len(ents))
	for i := range ents {
		result[i] = *models.FromInstrumentEntity(&ents[i])
	}
	return result, nil
}

func (s *MarketService) GetInstrument(ctx context.Context, id uuid.UUID) (*models.Instrument, error) {
	ent, err := s.instrumentStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.FromInstrumentEntity(ent), nil
}

func (s *MarketService) GetCandles(ctx context.Context, filter storage.CandleFilter) ([]models.Candle, error) {
	ents, err := s.candleStore.Query(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]models.Candle, len(ents))
	for i := range ents {
		result[i] = *models.FromCandleEntity(&ents[i])
	}
	return result, nil
}

func (s *MarketService) ProviderStatuses(ctx context.Context) []providers.ProviderStatus {
	return s.registry.Statuses(ctx)
}
