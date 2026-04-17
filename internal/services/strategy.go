package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

type StrategyService struct {
	strategyStore *storage.StrategyStore
}

func NewStrategyService(strategyStore *storage.StrategyStore) *StrategyService {
	return &StrategyService{strategyStore: strategyStore}
}

func (s *StrategyService) List(ctx context.Context) ([]models.Strategy, error) {
	ents, err := s.strategyStore.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]models.Strategy, len(ents))
	for i := range ents {
		result[i] = *models.FromStrategyEntity(&ents[i])
	}
	return result, nil
}

func (s *StrategyService) GetByID(ctx context.Context, id uuid.UUID) (*models.Strategy, error) {
	ent, err := s.strategyStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.FromStrategyEntity(ent), nil
}

func (s *StrategyService) Create(ctx context.Context, strategy *models.Strategy) error {
	return s.strategyStore.Create(ctx, strategy.ToEntity())
}

func (s *StrategyService) Update(ctx context.Context, strategy *models.Strategy) error {
	return s.strategyStore.Update(ctx, strategy.ToEntity())
}

func (s *StrategyService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.strategyStore.Delete(ctx, id)
}
