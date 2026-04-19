package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type StrategyService struct {
	strategyStore models.StrategyRepository
}

func NewStrategyService(strategyStore models.StrategyRepository) *StrategyService {
	return &StrategyService{strategyStore: strategyStore}
}

func (s *StrategyService) List(ctx context.Context) ([]models.Strategy, error) {
	return s.strategyStore.List(ctx)
}

func (s *StrategyService) GetByID(ctx context.Context, id uuid.UUID) (*models.Strategy, error) {
	return s.strategyStore.GetByID(ctx, id)
}

func (s *StrategyService) Create(ctx context.Context, strategy *models.Strategy) error {
	return s.strategyStore.Create(ctx, strategy)
}

func (s *StrategyService) Update(ctx context.Context, strategy *models.Strategy) error {
	return s.strategyStore.Update(ctx, strategy)
}

func (s *StrategyService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.strategyStore.Delete(ctx, id)
}
