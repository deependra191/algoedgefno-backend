package services

import (
	"context"
	"errors"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// Section key constants for the strategies list response.
const (
	SectionKeyBuiltin = "BUILTIN"
	SectionKeyCustom  = "CUSTOM"
)

// StrategySection is a grouping of strategies for the list endpoint.
type StrategySection struct {
	Key        string
	Strategies []StrategyListItem
}

// StrategyListItem pairs a built-in strategy with its most recent backtest run.
type StrategyListItem struct {
	Strategy     *models.BuiltinStrategy
	LastBacktest *models.BacktestRun
}

// StrategyDetail is the full strategy definition with dynamic maxDate and last backtest.
type StrategyDetail struct {
	Strategy     *models.BuiltinStrategy
	MaxDate      time.Time
	LastBacktest *models.BacktestRun
}

// StrategyService provides strategy listing and detail for the API layer.
// Phase 1 serves built-in strategies only. Phase 3 will add custom strategies.
type StrategyService struct {
	builtins     models.BuiltinStrategyLookup
	backtestRepo models.BacktestRepository
	candleRepo   models.CandleRepository
}

// NewStrategyService creates a StrategyService wired to the built-in registry and storage.
func NewStrategyService(
	builtins models.BuiltinStrategyLookup,
	backtestRepo models.BacktestRepository,
	candleRepo models.CandleRepository,
) *StrategyService {
	return &StrategyService{
		builtins:     builtins,
		backtestRepo: backtestRepo,
		candleRepo:   candleRepo,
	}
}

// ListSections returns BUILTIN and CUSTOM sections for the strategies list screen.
func (s *StrategyService) ListSections(ctx context.Context) ([]StrategySection, error) {
	all := s.builtins.All()
	items := make([]StrategyListItem, len(all))
	for i, b := range all {
		item := StrategyListItem{Strategy: b}
		run, err := s.backtestRepo.LatestCompletedBySlug(ctx, b.ID)
		if err != nil && !errors.Is(err, models.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			item.LastBacktest = run
		}
		items[i] = item
	}

	return []StrategySection{
		{
			Key:        SectionKeyBuiltin,
			Strategies: items,
		},
		{
			Key:        SectionKeyCustom,
			Strategies: []StrategyListItem{},
		},
	}, nil
}

// GetBySlug returns the full strategy detail for a built-in slug.
func (s *StrategyService) GetBySlug(ctx context.Context, slug string) (*StrategyDetail, error) {
	b, ok := s.builtins.Get(slug)
	if !ok {
		return nil, models.ErrNotFound
	}

	maxDate, err := s.candleRepo.MaxDate(ctx)
	if err != nil {
		return nil, err
	}

	detail := &StrategyDetail{
		Strategy: b,
		MaxDate:  maxDate,
	}

	run, err := s.backtestRepo.LatestCompletedBySlug(ctx, slug)
	if err != nil && !errors.Is(err, models.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		detail.LastBacktest = run
	}

	return detail, nil
}
