package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var (
	ErrStrategyNotFound    = errors.New("strategy not found")
	ErrNoInstrument        = errors.New("no instrument found for underlying")
	ErrNoCandleData        = errors.New("no candle data available")
	ErrInvalidUnderlying   = errors.New("invalid underlying for strategy")
)

const (
	underlyingInputKey   = "underlying"
	errMsgInternalError  = "internal error"
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
		return nil, ErrStrategyNotFound
	}

	if opts := underlyingOptions(builtin); len(opts) > 0 && !slices.Contains(opts, req.Underlying) {
		return nil, ErrInvalidUnderlying
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
		return s.failRun(ctx, run, fmt.Errorf("failed to fetch candle data: %w", err))
	}
	if len(candles) == 0 {
		return s.failRun(ctx, run, ErrNoCandleData)
	}

	result, err := s.engine.RunBacktest(engineStrategy, candles)
	if err != nil {
		return s.failRun(ctx, run, fmt.Errorf("engine error: %w", err))
	}

	return s.applyResult(ctx, run, result)
}

// GetByID returns a single backtest run by its UUID.
func (s *BacktestService) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	return s.backtestStore.GetByID(ctx, id)
}

// ListAll returns all backtest runs ordered newest first.
func (s *BacktestService) ListAll(ctx context.Context) ([]models.BacktestRun, error) {
	return s.backtestStore.ListAll(ctx)
}

// resolveInstrument finds the instrument matching the strategy's type and user's underlying.
// For futures types, it resolves to the continuous near-month instrument created during sync
// (e.g. FUTIDX → FUTIDX_CONT), so the backtest gets a single pre-stitched candle series.
func (s *BacktestService) resolveInstrument(ctx context.Context, instrumentType, underlying string) (*models.Instrument, error) {
	lookupType := instrumentType
	switch instrumentType {
	case models.InstrumentTypeFuturesIndex:
		lookupType = models.InstrumentTypeFuturesIndexCont
	case models.InstrumentTypeFuturesStock:
		lookupType = models.InstrumentTypeFuturesStockCont
	}

	instruments, err := s.instrumentStore.List(ctx, models.InstrumentFilter{
		InstrumentType: &lookupType,
		Underlying:     &underlying,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve instrument: %w", err)
	}
	if len(instruments) == 0 {
		return nil, ErrNoInstrument
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
		return nil, fmt.Errorf("failed to create backtest run: %w", err)
	}
	run.Status = models.BacktestRunning
	if err := s.backtestStore.UpdateStatus(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to update backtest status: %w", err)
	}
	return run, nil
}

// applyResult marshals trade data, stamps all result metrics onto the run,
// and persists the final COMPLETED state.
func (s *BacktestService) applyResult(ctx context.Context, run *models.BacktestRun, result *models.BacktestResult) (*models.BacktestRun, error) {
	tradesJSON, err := json.Marshal(result.Trades)
	if err != nil {
		return s.failRun(ctx, run, fmt.Errorf("failed to marshal trade results: %w", err))
	}
	run.Status = models.BacktestCompleted
	run.NetPnl = &result.NetPnL
	run.TotalTrades = &result.TotalTrades
	run.WinCount = &result.WinCount
	run.LossCount = &result.LossCount
	run.MaxDrawdown = &result.MaxDrawdown
	run.Trades = tradesJSON
	if err := s.backtestStore.UpdateResult(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to save backtest results: %w", err)
	}
	return run, nil
}

// failRun marks the run as FAILED and attempts to persist the state.
// The full error is logged; only a safe summary is stored in the DB to avoid
// leaking internal details (pgx errors, table names) if the column is ever exposed.
// The UpdateResult error is intentionally swallowed — the original cause is
// always returned to the caller regardless of persistence success.
func (s *BacktestService) failRun(ctx context.Context, run *models.BacktestRun, cause error) (*models.BacktestRun, error) {
	log.Printf("backtest %s failed: %v", run.ID, cause)
	safeMsg := safeErrorMessage(cause)
	run.Status = models.BacktestFailed
	run.ErrorMessage = &safeMsg
	_ = s.backtestStore.UpdateResult(ctx, run)
	return nil, cause
}

// safeErrorMessage returns a user-safe summary for known sentinel errors,
// falling back to a generic message for unexpected errors.
func safeErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrNoCandleData):
		return ErrNoCandleData.Error()
	case errors.Is(err, ErrNoInstrument):
		return ErrNoInstrument.Error()
	default:
		return errMsgInternalError
	}
}

// underlyingOptions returns the allowed values for the "underlying" input
// declared by the strategy, or nil if the strategy has no such input.
func underlyingOptions(b *models.BuiltinStrategy) []string {
	for _, inp := range b.Inputs {
		if inp.Key == underlyingInputKey {
			return inp.Options
		}
	}
	return nil
}
