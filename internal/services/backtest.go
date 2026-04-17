package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/engine"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

type BacktestService struct {
	backtestStore   *storage.BacktestStore
	strategyStore   *storage.StrategyStore
	candleStore     *storage.CandleStore
	instrumentStore *storage.InstrumentStore
}

func NewBacktestService(
	backtestStore *storage.BacktestStore,
	strategyStore *storage.StrategyStore,
	candleStore *storage.CandleStore,
	instrumentStore *storage.InstrumentStore,
) *BacktestService {
	return &BacktestService{
		backtestStore:   backtestStore,
		strategyStore:   strategyStore,
		candleStore:     candleStore,
		instrumentStore: instrumentStore,
	}
}

type BacktestRequest struct {
	StrategyID   uuid.UUID
	InstrumentID uuid.UUID
	From         time.Time
	To           time.Time
	Interval     string
}

func (s *BacktestService) Submit(ctx context.Context, req BacktestRequest) (*models.BacktestRun, error) {
	_, err := s.strategyStore.GetByID(ctx, req.StrategyID)
	if err != nil {
		return nil, errors.New("strategy not found")
	}

	inst, err := s.instrumentStore.GetByID(ctx, req.InstrumentID)
	if err != nil {
		return nil, errors.New("instrument not found")
	}

	run := &models.BacktestRun{
		ID:              uuid.New(),
		StrategyID:      req.StrategyID,
		InstrumentToken: inst.Symbol,
		FromTs:          req.From,
		ToTs:            req.To,
		CandleInterval:  req.Interval,
		Status:          models.BacktestPending,
	}

	if err := s.backtestStore.Create(ctx, run.ToEntity()); err != nil {
		return nil, errors.New("failed to create backtest run")
	}

	run.Status = models.BacktestRunning
	if err := s.backtestStore.UpdateResult(ctx, run.ToEntity()); err != nil {
		return nil, errors.New("failed to update backtest status")
	}

	candleEnts, err := s.candleStore.Query(ctx, storage.CandleFilter{
		InstrumentID: req.InstrumentID,
		From:         req.From,
		To:           req.To,
		Interval:     req.Interval,
	})
	if err != nil {
		return s.failRun(ctx, run, "failed to fetch candle data")
	}
	if len(candleEnts) == 0 {
		return s.failRun(ctx, run, "no candle data available")
	}

	candles := make([]models.Candle, len(candleEnts))
	for i := range candleEnts {
		candles[i] = *models.FromCandleEntity(&candleEnts[i])
	}

	strategy, err := s.strategyStore.GetByID(ctx, req.StrategyID)
	if err != nil {
		return s.failRun(ctx, run, "failed to reload strategy")
	}
	strat := models.FromStrategyEntity(strategy)

	result, err := engine.RunBacktest(strat, candles)
	if err != nil {
		return s.failRun(ctx, run, err.Error())
	}

	tradesJSON, _ := json.Marshal(result.Trades)
	run.Status = models.BacktestCompleted
	run.NetPnl = &result.NetPnL
	run.TotalTrades = &result.TotalTrades
	run.WinCount = &result.WinCount
	run.LossCount = &result.LossCount
	run.MaxDrawdown = &result.MaxDrawdown
	run.Trades = tradesJSON

	ent := run.ToEntity()
	if err := s.backtestStore.UpdateResult(ctx, ent); err != nil {
		return nil, errors.New("failed to save backtest results")
	}

	return models.FromBacktestRunEntity(ent), nil
}

func (s *BacktestService) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	ent, err := s.backtestStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return models.FromBacktestRunEntity(ent), nil
}

func (s *BacktestService) ListByStrategy(ctx context.Context, strategyID uuid.UUID) ([]models.BacktestRun, error) {
	ents, err := s.backtestStore.ListByStrategy(ctx, strategyID)
	if err != nil {
		return nil, err
	}
	result := make([]models.BacktestRun, len(ents))
	for i := range ents {
		result[i] = *models.FromBacktestRunEntity(&ents[i])
	}
	return result, nil
}

func (s *BacktestService) failRun(ctx context.Context, run *models.BacktestRun, errMsg string) (*models.BacktestRun, error) {
	run.Status = models.BacktestFailed
	run.ErrorMessage = &errMsg
	_ = s.backtestStore.UpdateResult(ctx, run.ToEntity())
	return nil, errors.New(errMsg)
}
