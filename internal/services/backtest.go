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
	builtins        models.BuiltinStrategyLookup
	candleStore     models.CandleRepository
	instrumentStore models.InstrumentRepository
	engine          models.BacktestEngine
}

// NewBacktestService wires the backtest lifecycle to storage, registry, and engine.
func NewBacktestService(
	backtestStore models.BacktestRepository,
	builtins models.BuiltinStrategyLookup,
	candleStore models.CandleRepository,
	instrumentStore models.InstrumentRepository,
	engine models.BacktestEngine,
) *BacktestService {
	return &BacktestService{
		backtestStore:   backtestStore,
		builtins:        builtins,
		candleStore:     candleStore,
		instrumentStore: instrumentStore,
		engine:          engine,
	}
}

// BacktestRequest carries the user-supplied inputs for a backtest submission.
type BacktestRequest struct {
	StrategySlug string
	Underlying   string
	From         time.Time
	To           time.Time
	Lots         int
	Capital      float64
}

// Submit validates the request, runs the backtest engine synchronously, and
// persists the result. Returns the completed (or failed) BacktestRun.
func (s *BacktestService) Submit(ctx context.Context, req BacktestRequest) (*models.BacktestRun, error) {
	builtin, ok := s.builtins.Get(req.StrategySlug)
	if !ok {
		return nil, errors.New("strategy not found")
	}

	inst, err := s.resolveInstrument(ctx, builtin.InstrumentType, req.Underlying)
	if err != nil {
		return nil, err
	}

	engineStrategy := &models.Strategy{
		Name:               builtin.Name,
		Description:        builtin.Description,
		Underlying:         req.Underlying,
		InstrumentType:     builtin.InstrumentType,
		ExpiryRule:         builtin.ExpiryRule,
		EntryConditionType: builtin.EntryConditionType,
		TargetPct:          builtin.TargetPct,
		StopLossPct:        builtin.StopLossPct,
		TimeExitMinutes:    builtin.TimeExitMinutes,
		LotSize:            inst.LotSize,
		NumberOfLots:       req.Lots,
	}

	run, err := s.createAndStartRun(ctx, inst, builtin, req)
	if err != nil {
		return nil, err
	}

	candles, err := s.candleStore.Query(ctx, models.CandleFilter{
		InstrumentID: inst.ID,
		From:         req.From,
		To:           req.To,
		Interval:     builtin.CandleInterval,
	})
	if err != nil {
		return s.failRun(ctx, run, "failed to fetch candle data")
	}
	if len(candles) == 0 {
		return s.failRun(ctx, run, "no candle data available")
	}

	result, err := s.engine.RunBacktest(engineStrategy, candles)
	if err != nil {
		return s.failRun(ctx, run, err.Error())
	}

	return s.applyResult(ctx, run, result)
}

// GetByID returns a single backtest run by its UUID.
func (s *BacktestService) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	return s.backtestStore.GetByID(ctx, id)
}

// resolveInstrument finds the first instrument matching the strategy's type and user's underlying.
func (s *BacktestService) resolveInstrument(ctx context.Context, instrumentType, underlying string) (*models.Instrument, error) {
	instruments, err := s.instrumentStore.List(ctx, models.InstrumentFilter{
		InstrumentType: &instrumentType,
		Underlying:     &underlying,
	})
	if err != nil {
		return nil, errors.New("failed to resolve instrument")
	}
	if len(instruments) == 0 {
		return nil, errors.New("no instrument found for underlying")
	}
	return &instruments[0], nil
}

// createAndStartRun builds the BacktestRun record, persists it, and transitions
// it to RUNNING. Returns the persisted run ready for candle fetching.
func (s *BacktestService) createAndStartRun(ctx context.Context, inst *models.Instrument, builtin *models.BuiltinStrategy, req BacktestRequest) (*models.BacktestRun, error) {
	run := &models.BacktestRun{
		ID:              uuid.New(),
		StrategySlug:    &req.StrategySlug,
		InstrumentToken: inst.Symbol,
		FromTs:          req.From,
		ToTs:            req.To,
		CandleInterval:  builtin.CandleInterval,
		Status:          models.BacktestPending,
		Capital:         &req.Capital,
		Lots:            &req.Lots,
		Underlying:      &req.Underlying,
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
