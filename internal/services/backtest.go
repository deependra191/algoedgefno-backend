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

const (
	// NoDayLimit and NoCandleLimit are sentinel values for BacktestLimits fields,
	// meaning the corresponding limit is not enforced.
	NoDayLimit    = 0
	NoCandleLimit = 0

	oneDay = 24 * time.Hour
)

var (
	ErrStrategyNotFound            = errors.New("strategy not found")
	ErrNoInstrument                = errors.New("no instrument found for underlying")
	ErrNoCandleData                = errors.New("no candle data available")
	ErrInvalidUnderlying           = errors.New("invalid underlying for strategy")
	ErrInvalidStrategySources      = errors.New("invalid strategy source declaration")
	ErrBacktestDisabled            = errors.New("backtests are disabled")
	ErrBacktestDateRangeExceeded   = errors.New("backtest date range exceeds maximum allowed days")
	ErrBacktestCandleCountExceeded = errors.New("backtest candle count exceeds maximum")
)

// BacktestLimits defines the operational constraints for backtest submission.
// All fields are read once at service construction; changing env values requires
// a process restart to take effect.
//
// MaxDays of 0 means no date-range limit; MaxCandles of 0 means no candle-count limit.
// MaxCandles is a post-fetch engine-safety guard — it does not prevent the DB read.
// Use MaxDays to constrain DB read cost.
type BacktestLimits struct {
	// BacktestsEnabled gates all new backtest submissions. Running backtests are not cancelled.
	BacktestsEnabled bool
	MaxDays          int
	MaxCandles       int
}

const (
	errMsgInternalError                   = "internal error"
	errMsgAmbiguousInstrumentResolution   = "ambiguous instrument resolution"
	errMsgUnsupportedInstrumentResolution = "unsupported instrument kind for v1 source resolution"
)

// BacktestService orchestrates the full backtest lifecycle: create a run record,
// fetch candles, invoke the engine, and persist results.
type BacktestService struct {
	backtestStore   models.BacktestRepository
	builtins        models.BuiltinStrategyLookup
	candleStore     models.CandleRepository
	instrumentStore models.InstrumentRepository
	engine          models.BacktestEngine
	limits          BacktestLimits
	// launch fires the background execution function. Defaults to a goroutine;
	// tests replace this with a synchronous caller to observe final state.
	launch func(func())
}

// NewBacktestService wires the backtest lifecycle to storage, registry, and engine.
func NewBacktestService(
	backtestStore models.BacktestRepository,
	builtins models.BuiltinStrategyLookup,
	candleStore models.CandleRepository,
	instrumentStore models.InstrumentRepository,
	engine models.BacktestEngine,
	limits BacktestLimits,
) *BacktestService {
	return &BacktestService{
		backtestStore:   backtestStore,
		builtins:        builtins,
		candleStore:     candleStore,
		instrumentStore: instrumentStore,
		engine:          engine,
		limits:          limits,
		launch:          func(fn func()) { go fn() },
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

// Submit validates the request and creates a RUNNING backtest run, then fires the
// engine asynchronously. The caller receives the run in RUNNING state immediately;
// poll GetByID until status transitions to COMPLETED or FAILED.
// Candle availability is checked synchronously so the caller gets an immediate error
// if no data exists for the requested range.
func (s *BacktestService) Submit(ctx context.Context, req BacktestRequest) (*models.BacktestRun, error) {
	if !s.limits.BacktestsEnabled {
		return nil, ErrBacktestDisabled
	}
	if s.limits.MaxDays != NoDayLimit {
		// +1 makes the range calendar-inclusive: From=Jan 1, To=Jan 31 → 31 days.
		rangeDays := int(req.To.Sub(req.From)/oneDay) + 1
		if rangeDays > s.limits.MaxDays {
			return nil, ErrBacktestDateRangeExceeded
		}
	}

	builtin, ok := s.builtins.Get(req.StrategySlug)
	if !ok {
		return nil, ErrStrategyNotFound
	}

	if opts := underlyingOptions(builtin); len(opts) > 0 && !slices.Contains(opts, req.Underlying) {
		return nil, ErrInvalidUnderlying
	}

	if builtin.SourceResolver == nil {
		return nil, ErrInvalidStrategySources
	}
	params := models.StrategyParams{Underlying: req.Underlying}
	sources := builtin.SourceResolver.Sources(params)
	if err := models.ValidateStrategySourceIntervals(sources, builtin.CandleInterval); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidStrategySources, err)
	}

	signalInst, err := s.resolveInstrumentSpec(ctx, sources.Signal, req.Underlying, req.From)
	if err != nil {
		return nil, err
	}
	tradeInst, err := s.resolveInstrumentSpec(ctx, sources.Trade, req.Underlying, req.From)
	if err != nil {
		return nil, err
	}

	inputs, err := s.fetchEngineInputs(ctx, signalInst, tradeInst, builtin.CandleInterval, req.From, req.To)
	if err != nil {
		return nil, err
	}
	if s.limits.MaxCandles != NoCandleLimit {
		count := max(len(inputs.SignalCandles), len(inputs.TradeCandles))
		if count > s.limits.MaxCandles {
			return nil, ErrBacktestCandleCountExceeded
		}
	}

	// Runs intentionally snapshot the current trade lot size at submission time.
	// Historical re-runs after exchange lot-size changes need explicit lot-size history.
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
		LotSize:            tradeInst.LotSize,
		NumberOfLots:       req.Lots,
	}

	run, err := s.createAndStartRun(ctx, signalInst, tradeInst, builtin, req)
	if err != nil {
		return nil, err
	}

	// Return a snapshot of the RUNNING run before the goroutine can mutate it.
	snapshot := *run
	s.launch(func() { s.executeRun(run, engineStrategy, inputs, req.Capital) })

	return &snapshot, nil
}

// GetByID returns a single backtest run by its UUID. Does not include the trades blob.
// StrategyName is populated from the builtins registry when the run references a built-in slug.
func (s *BacktestService) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	run, err := s.backtestStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.populateStrategyName(run)
	return run, nil
}

// GetByIDWithTrades returns a single backtest run by its UUID including the raw trades blob.
// Use this only for the paginated trades endpoint.
func (s *BacktestService) GetByIDWithTrades(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	return s.backtestStore.GetByIDWithTrades(ctx, id)
}

// ListCompleted returns completed backtest runs ordered by most recent completion first.
func (s *BacktestService) ListCompleted(ctx context.Context, page, limit int) ([]models.BacktestRun, int, error) {
	runs, total, err := s.backtestStore.ListCompleted(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}
	for i := range runs {
		s.populateStrategyName(&runs[i])
	}
	return runs, total, nil
}

func (s *BacktestService) populateStrategyName(run *models.BacktestRun) {
	if run.StrategySlug == nil {
		return
	}
	if b, ok := s.builtins.Get(*run.StrategySlug); ok {
		run.StrategyName = &b.Name
	}
}

// executeRun runs the engine and persists the result. It is called in a goroutine
// and uses context.Background() since the originating HTTP request context will
// have been cancelled by the time this executes.
func (s *BacktestService) executeRun(run *models.BacktestRun, strategy *models.Strategy, inputs models.EngineInputs, capital float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("backtest %s: engine panic: %v", run.ID, r)
			s.failRun(context.Background(), run, fmt.Errorf("engine panic: %v", r))
		}
	}()
	ctx := context.Background()
	result, err := s.engine.RunBacktest(strategy, inputs, capital)
	if err != nil {
		s.failRun(ctx, run, fmt.Errorf("engine error: %w", err))
		return
	}
	if err := s.applyResult(ctx, run, result, capital); err != nil {
		log.Printf("backtest %s: failed to persist result: %v", run.ID, err)
	}
}

// resolveInstrumentSpec finds the instrument matching the declared source kind and user's underlying.
// V1 source resolution only supports kinds that resolve to one concrete instrument per underlying.
// FUT*_CONT is treated as a synthetic continuous execution stream; explicit roll exits/re-entries
// can be added later as engine inputs without moving instrument taxonomy into the engine.
func (s *BacktestService) resolveInstrumentSpec(ctx context.Context, spec models.InstrumentSpec, underlying string, _ time.Time) (*models.Instrument, error) {
	if !supportsSingleInstrumentResolution(spec.Kind) {
		return nil, fmt.Errorf("%w: %s: %s", ErrInvalidStrategySources, errMsgUnsupportedInstrumentResolution, spec.Kind)
	}
	lookupType, err := instrumentTypeForKind(spec.Kind)
	if err != nil {
		return nil, err
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
	if len(instruments) > 1 {
		return nil, fmt.Errorf("%w: %s for %s %s", ErrInvalidStrategySources, errMsgAmbiguousInstrumentResolution, underlying, lookupType)
	}
	return &instruments[0], nil
}

func instrumentTypeForKind(kind models.InstrumentKind) (string, error) {
	instrumentType := string(kind)
	switch instrumentType {
	case models.InstrumentTypeIndex,
		models.InstrumentTypeEquity,
		models.InstrumentTypeFuturesIndex,
		models.InstrumentTypeFuturesStock,
		models.InstrumentTypeFuturesIndexCont,
		models.InstrumentTypeFuturesStockCont,
		models.InstrumentTypeOptionsIndex,
		models.InstrumentTypeOptionsStock:
		return instrumentType, nil
	default:
		return "", fmt.Errorf("%w: unknown instrument kind %s", ErrInvalidStrategySources, kind)
	}
}

func supportsSingleInstrumentResolution(kind models.InstrumentKind) bool {
	switch kind {
	case models.InstrumentKindIndex,
		models.InstrumentKindEquity,
		models.InstrumentKindFuturesIndexContinuous,
		models.InstrumentKindFuturesStockContinuous:
		return true
	default:
		return false
	}
}

func (s *BacktestService) fetchEngineInputs(
	ctx context.Context,
	signalInst *models.Instrument,
	tradeInst *models.Instrument,
	interval string,
	from time.Time,
	to time.Time,
) (models.EngineInputs, error) {
	signalCandles, err := s.fetchCandles(ctx, signalInst.ID, interval, from, to)
	if err != nil {
		return models.EngineInputs{}, fmt.Errorf("failed to fetch signal candle data: %w", err)
	}
	if len(signalCandles) == 0 {
		return models.EngineInputs{}, ErrNoCandleData
	}

	tradeCandles, err := s.fetchCandles(ctx, tradeInst.ID, interval, from, to)
	if err != nil {
		return models.EngineInputs{}, fmt.Errorf("failed to fetch trade candle data: %w", err)
	}
	if len(tradeCandles) == 0 {
		return models.EngineInputs{}, ErrNoCandleData
	}

	return models.EngineInputs{
		Interval:      interval,
		SignalCandles: signalCandles,
		TradeCandles:  tradeCandles,
	}, nil
}

func (s *BacktestService) fetchCandles(ctx context.Context, instrumentID uuid.UUID, interval string, from time.Time, to time.Time) ([]models.Candle, error) {
	return s.candleStore.Query(ctx, models.CandleFilter{
		InstrumentID: instrumentID,
		From:         from,
		To:           to,
		Interval:     interval,
	})
}

// createAndStartRun builds the BacktestRun record, persists it, and transitions
// it to RUNNING. Returns the persisted run ready for engine execution.
func (s *BacktestService) createAndStartRun(
	ctx context.Context,
	signalInst *models.Instrument,
	tradeInst *models.Instrument,
	builtin *models.BuiltinStrategy,
	req BacktestRequest,
) (*models.BacktestRun, error) {
	signalToken := signalInst.Symbol
	run := &models.BacktestRun{
		ID:                    uuid.New(),
		StrategySlug:          &req.StrategySlug,
		InstrumentToken:       tradeInst.Symbol,
		SignalInstrumentToken: &signalToken,
		FromTs:                req.From,
		ToTs:                  req.To,
		CandleInterval:        builtin.CandleInterval,
		Status:                models.BacktestPending,
		Capital:               &req.Capital,
		Lots:                  &req.Lots,
		Underlying:            &req.Underlying,
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

// applyResult marshals trade data, computes derived stats and chart data,
// stamps all metrics onto the run, and persists the final COMPLETED state.
func (s *BacktestService) applyResult(ctx context.Context, run *models.BacktestRun, result *models.BacktestResult, capital float64) error {
	tradesJSON, err := json.Marshal(result.Trades)
	if err != nil {
		_, cause := s.failRun(ctx, run, fmt.Errorf("failed to marshal trade results: %w", err))
		return cause
	}
	run.Status = models.BacktestCompleted
	run.NetPnl = &result.NetPnL
	run.TotalTrades = &result.TotalTrades
	run.WinCount = &result.WinCount
	run.LossCount = &result.LossCount
	run.MaxDrawdown = &result.MaxDrawdown
	run.Trades = tradesJSON
	run.ResultStats = s.engine.ComputeTradeStats(result.Trades, run.FromTs, run.ToTs)
	run.ChartData = s.engine.BuildChartData(result.Trades, capital)
	if err := s.backtestStore.UpdateResult(ctx, run); err != nil {
		return fmt.Errorf("failed to save backtest results: %w", err)
	}
	return nil
}

// failRun marks the run as FAILED and attempts to persist the state.
// The full error is logged; only a safe summary is stored in the DB to avoid
// leaking internal details if the column is ever exposed.
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
		if inp.Key == models.StrategyInputKeyUnderlying {
			return inp.Options
		}
	}
	return nil
}
