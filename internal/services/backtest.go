package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// BacktestService orchestrates the full backtest lifecycle: create a run record,
// fetch candles, invoke the engine, and persist results.
type BacktestService struct {
	backtestStore   models.BacktestRepository
	strategyStore   models.StrategyRepository
	candleStore     models.CandleRepository
	instrumentStore models.InstrumentRepository
	engine          models.BacktestEngine
}

func NewBacktestService(
	backtestStore models.BacktestRepository,
	strategyStore models.StrategyRepository,
	candleStore models.CandleRepository,
	instrumentStore models.InstrumentRepository,
	engine models.BacktestEngine,
) *BacktestService {
	return &BacktestService{
		backtestStore:   backtestStore,
		strategyStore:   strategyStore,
		candleStore:     candleStore,
		instrumentStore: instrumentStore,
		engine:          engine,
	}
}

type BacktestRequest struct {
	StrategyID   uuid.UUID
	InstrumentID uuid.UUID
	From         time.Time
	To           time.Time
	Interval     string
}

// Submit validates the request, runs the backtest engine synchronously, and
// persists the result. Returns the completed (or failed) BacktestRun.
func (s *BacktestService) Submit(ctx context.Context, req BacktestRequest) (*models.BacktestRun, error) {
	inst, strat, err := s.validateInputs(ctx, req)
	if err != nil {
		return nil, err
	}

	run, err := s.createAndStartRun(ctx, inst, req)
	if err != nil {
		return nil, err
	}

	candles, err := s.candleStore.Query(ctx, models.CandleFilter{
		InstrumentID: req.InstrumentID,
		From:         req.From,
		To:           req.To,
		Interval:     req.Interval,
	})
	if err != nil {
		return s.failRun(ctx, run, "failed to fetch candle data")
	}
	if len(candles) == 0 {
		return s.failRun(ctx, run, "no candle data available")
	}

	result, err := s.engine.RunBacktest(strat, candles)
	if err != nil {
		return s.failRun(ctx, run, err.Error())
	}

	return s.applyResult(ctx, run, result)
}

func (s *BacktestService) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	return s.backtestStore.GetByID(ctx, id)
}

func (s *BacktestService) ListByStrategy(ctx context.Context, strategyID uuid.UUID) ([]models.BacktestRun, error) {
	return s.backtestStore.ListByStrategy(ctx, strategyID)
}

// validateInputs loads and validates both the strategy and instrument.
// Returns both to avoid a second fetch later in the lifecycle.
func (s *BacktestService) validateInputs(ctx context.Context, req BacktestRequest) (*models.Instrument, *models.Strategy, error) {
	strat, err := s.strategyStore.GetByID(ctx, req.StrategyID)
	if err != nil {
		return nil, nil, errors.New("strategy not found")
	}
	inst, err := s.instrumentStore.GetByID(ctx, req.InstrumentID)
	if err != nil {
		return nil, nil, errors.New("instrument not found")
	}
	return inst, strat, nil
}

// createAndStartRun builds the BacktestRun record, persists it, and transitions
// it to RUNNING. Returns the persisted run ready for candle fetching.
func (s *BacktestService) createAndStartRun(ctx context.Context, inst *models.Instrument, req BacktestRequest) (*models.BacktestRun, error) {
	run := &models.BacktestRun{
		ID:              uuid.New(),
		StrategyID:      req.StrategyID,
		InstrumentToken: inst.Symbol,
		FromTs:          req.From,
		ToTs:            req.To,
		CandleInterval:  req.Interval,
		Status:          models.BacktestPending,
	}
	if err := s.backtestStore.Create(ctx, run); err != nil {
		return nil, errors.New("failed to create backtest run")
	}
	run.Status = models.BacktestRunning
	if err := s.backtestStore.UpdateStatus(ctx, run); err != nil {
		return nil, errors.New("failed to update backtest status")
	}
	return run, nil
}

// applyResult marshals trade data, stamps all result metrics onto the run,
// and persists the final COMPLETED state.
func (s *BacktestService) applyResult(ctx context.Context, run *models.BacktestRun, result *models.BacktestResult) (*models.BacktestRun, error) {
	tradesJSON, err := json.Marshal(result.Trades)
	if err != nil {
		return s.failRun(ctx, run, "failed to marshal trade results")
	}
	run.Status = models.BacktestCompleted
	run.NetPnl = &result.NetPnL
	run.TotalTrades = &result.TotalTrades
	run.WinCount = &result.WinCount
	run.LossCount = &result.LossCount
	run.MaxDrawdown = &result.MaxDrawdown
	run.Trades = tradesJSON
	if err := s.backtestStore.UpdateResult(ctx, run); err != nil {
		return nil, errors.New("failed to save backtest results")
	}
	return run, nil
}

// failRun marks the run as FAILED and attempts to persist the state.
// The UpdateResult error is intentionally swallowed — the original errMsg is
// always returned to the caller regardless of persistence success.
func (s *BacktestService) failRun(ctx context.Context, run *models.BacktestRun, errMsg string) (*models.BacktestRun, error) {
	run.Status = models.BacktestFailed
	run.ErrorMessage = &errMsg
	_ = s.backtestStore.UpdateResult(ctx, run)
	return nil, errors.New(errMsg)
}
