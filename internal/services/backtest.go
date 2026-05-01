package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var (
	ErrStrategyNotFound = errors.New("strategy not found")
	ErrNoInstrument     = errors.New("no instrument found for underlying")
	ErrNoCandleData     = errors.New("no candle data available")
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

	contracts, err := s.resolveContracts(ctx, builtin.InstrumentType, req.Underlying, req.From, req.To)
	if err != nil {
		return nil, err
	}

	inst := &contracts[0]

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

	candles, err := s.stitchCandles(ctx, contracts, req.From, req.To, builtin.CandleInterval)
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

// isFuturesType returns true for instrument types that represent monthly futures contracts.
func isFuturesType(instrumentType string) bool {
	return instrumentType == models.InstrumentTypeFuturesIndex ||
		instrumentType == models.InstrumentTypeFuturesStock
}

// resolveContracts returns the ordered sequence of near-month contracts covering [from, to].
// For non-futures types it returns a single-element slice (existing behaviour).
// For futures types it sets only ExpiryFrom — it intentionally does NOT set ExpiryTo
// because the last contract in the range will have an expiry beyond `to` (e.g. if the
// user requests up to 2024-03-15, the contract expiring 2024-03-28 must still be
// included). The stitchCandles loop handles the upper boundary via min(to, contract.Expiry).
func (s *BacktestService) resolveContracts(ctx context.Context, instrumentType, underlying string, from, to time.Time) ([]models.Instrument, error) {
	filter := models.InstrumentFilter{
		InstrumentType: &instrumentType,
		Underlying:     &underlying,
	}

	if isFuturesType(instrumentType) {
		filter.ExpiryFrom = &from
	}

	instruments, err := s.instrumentStore.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve instrument: %w", err)
	}
	if len(instruments) == 0 {
		return nil, ErrNoInstrument
	}
	return instruments, nil
}

// stitchCandles fetches candles from each contract for only the period it was
// the active near-month contract, then concatenates them in chronological order.
// For a single contract (non-futures), this is equivalent to a simple query.
func (s *BacktestService) stitchCandles(ctx context.Context, contracts []models.Instrument, from, to time.Time, interval string) ([]models.Candle, error) {
	if len(contracts) == 1 {
		return s.candleStore.Query(ctx, models.CandleFilter{
			InstrumentID: contracts[0].ID,
			From:         from,
			To:           to,
			Interval:     interval,
		})
	}

	var allCandles []models.Candle
	for i, contract := range contracts {
		segFrom := from
		if i > 0 && contracts[i-1].Expiry != nil {
			dayAfterPrevExpiry := contracts[i-1].Expiry.AddDate(0, 0, 1)
			if dayAfterPrevExpiry.After(segFrom) {
				segFrom = dayAfterPrevExpiry
			}
		}

		segTo := to
		if contract.Expiry != nil && contract.Expiry.Before(to) {
			segTo = *contract.Expiry
		}

		if segFrom.After(segTo) {
			continue
		}

		candles, err := s.candleStore.Query(ctx, models.CandleFilter{
			InstrumentID: contract.ID,
			From:         segFrom,
			To:           segTo,
			Interval:     interval,
		})
		if err != nil {
			return nil, err
		}
		allCandles = append(allCandles, candles...)
	}
	return allCandles, nil
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
// The UpdateResult error is intentionally swallowed — the original cause is
// always returned to the caller regardless of persistence success.
func (s *BacktestService) failRun(ctx context.Context, run *models.BacktestRun, cause error) (*models.BacktestRun, error) {
	msg := cause.Error()
	run.Status = models.BacktestFailed
	run.ErrorMessage = &msg
	_ = s.backtestStore.UpdateResult(ctx, run)
	return nil, cause
}
