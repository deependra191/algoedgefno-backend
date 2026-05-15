package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// -- hand-rolled mocks --

type mockBacktestRepo struct {
	createErr       error
	updateStatusErr error
	updateResultErr error
	getByIDResult   *models.BacktestRun
	getByIDErr      error
	listResult      []models.BacktestRun
	listErr         error
	listTotal       int

	capturedCreate       *models.BacktestRun
	capturedUpdateStatus []*models.BacktestRun
	capturedUpdateResult []*models.BacktestRun
	capturedListPage     int
	capturedListLimit    int
}

func (m *mockBacktestRepo) Create(_ context.Context, run *models.BacktestRun) error {
	m.capturedCreate = run
	return m.createErr
}
func (m *mockBacktestRepo) UpdateStatus(_ context.Context, run *models.BacktestRun) error {
	cp := *run
	m.capturedUpdateStatus = append(m.capturedUpdateStatus, &cp)
	return m.updateStatusErr
}
func (m *mockBacktestRepo) UpdateResult(_ context.Context, run *models.BacktestRun) error {
	cp := *run
	m.capturedUpdateResult = append(m.capturedUpdateResult, &cp)
	return m.updateResultErr
}
func (m *mockBacktestRepo) GetByID(_ context.Context, _ uuid.UUID) (*models.BacktestRun, error) {
	return m.getByIDResult, m.getByIDErr
}
func (m *mockBacktestRepo) ListByStrategy(_ context.Context, _ uuid.UUID) ([]models.BacktestRun, error) {
	return m.listResult, m.listErr
}
func (m *mockBacktestRepo) LatestCompletedBySlug(_ context.Context, _ string) (*models.BacktestRun, error) {
	return nil, models.ErrNotFound
}
func (m *mockBacktestRepo) GetByIDWithTrades(_ context.Context, _ uuid.UUID) (*models.BacktestRun, error) {
	return m.getByIDResult, m.getByIDErr
}
func (m *mockBacktestRepo) ListCompleted(_ context.Context, page, limit int) ([]models.BacktestRun, int, error) {
	m.capturedListPage = page
	m.capturedListLimit = limit
	total := m.listTotal
	if total == 0 {
		total = len(m.listResult)
	}
	return m.listResult, total, m.listErr
}

type mockBuiltinLookup struct {
	strategies map[string]*models.BuiltinStrategy
	order      []string
}

func (m *mockBuiltinLookup) Get(slug string) (*models.BuiltinStrategy, bool) {
	s, ok := m.strategies[slug]
	return s, ok
}
func (m *mockBuiltinLookup) All() []*models.BuiltinStrategy {
	result := make([]*models.BuiltinStrategy, 0, len(m.order))
	for _, slug := range m.order {
		result = append(result, m.strategies[slug])
	}
	return result
}

type mockCandleRepo struct {
	resultByInstrument map[uuid.UUID][]models.Candle
	result             []models.Candle
	err                error
	queries            []models.CandleFilter
}

func (m *mockCandleRepo) Query(_ context.Context, f models.CandleFilter) ([]models.Candle, error) {
	m.queries = append(m.queries, f)
	if m.resultByInstrument != nil {
		return m.resultByInstrument[f.InstrumentID], m.err
	}
	return m.result, m.err
}
func (m *mockCandleRepo) InsertBatchIgnoreDuplicates(_ context.Context, _ []models.Candle) (int64, error) {
	return 0, nil
}
func (m *mockCandleRepo) MaxDate(_ context.Context) (time.Time, error) {
	return time.Time{}, nil
}

type mockInstrumentRepo struct {
	listResult []models.Instrument
	listErr    error
}

func (m *mockInstrumentRepo) GetByID(_ context.Context, _ uuid.UUID) (*models.Instrument, error) {
	return nil, models.ErrNotFound
}
func (m *mockInstrumentRepo) List(_ context.Context, f models.InstrumentFilter) ([]models.Instrument, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	result := make([]models.Instrument, 0, len(m.listResult))
	for _, inst := range m.listResult {
		if f.InstrumentType != nil && inst.InstrumentType != *f.InstrumentType {
			continue
		}
		if f.Underlying != nil {
			if inst.Underlying == nil || *inst.Underlying != *f.Underlying {
				continue
			}
		}
		result = append(result, inst)
	}
	return result, nil
}
func (m *mockInstrumentRepo) UpsertBatch(_ context.Context, _ []models.Instrument) error {
	return nil
}

type mockEngine struct {
	result         *models.BacktestResult
	err            error
	capturedInputs models.EngineInputs
}

func (m *mockEngine) RunBacktest(_ *models.Strategy, inputs models.EngineInputs, _ float64) (*models.BacktestResult, error) {
	m.capturedInputs = inputs
	return m.result, m.err
}
func (m *mockEngine) ComputeTradeStats(trades []models.Trade, from, to time.Time) *models.TradeStats {
	return &models.TradeStats{}
}
func (m *mockEngine) BuildChartData(_ []models.Trade, _ float64, _, _ time.Time) *models.ChartData {
	return &models.ChartData{}
}

// -- helpers --

const (
	testSlug                    = "ma_crossover"
	unsupportedStrategyInterval = "1m"
)

func defaultBuiltin() *models.BuiltinStrategy {
	return &models.BuiltinStrategy{
		ID:                 testSlug,
		Name:               "MA Crossover",
		EntryConditionType: models.EntryConditionMACrossover,
		InstrumentType:     models.InstrumentTypeFuturesIndex,
		ExpiryRule:         models.ExpiryRuleCurrentMonth,
		CandleInterval:     models.CandleInterval1D,
		SourceResolver: fixedTestSourceResolver{
			sources: models.StrategySources{
				Signal: models.InstrumentSpec{Kind: models.InstrumentKindIndex},
				Trade:  models.InstrumentSpec{Kind: models.InstrumentKindFuturesIndexContinuous},
			},
		},
		Inputs: []models.StrategyInput{
			{
				Key:     models.StrategyInputKeyUnderlying,
				Type:    models.InputTypeSelect,
				Options: []string{models.UnderlyingNifty, models.UnderlyingBankNifty, models.UnderlyingFinNifty},
			},
		},
	}
}

type fixedTestSourceResolver struct {
	sources models.StrategySources
}

func (r fixedTestSourceResolver) Sources(_ models.StrategyParams) models.StrategySources {
	return r.sources
}

func defaultLookup() *mockBuiltinLookup {
	return &mockBuiltinLookup{
		strategies: map[string]*models.BuiltinStrategy{testSlug: defaultBuiltin()},
		order:      []string{testSlug},
	}
}

func emptyLookup() *mockBuiltinLookup {
	return &mockBuiltinLookup{strategies: map[string]*models.BuiltinStrategy{}}
}

func defaultInstruments() []models.Instrument {
	underlying := models.UnderlyingNifty
	return []models.Instrument{
		{
			ID:             uuid.New(),
			Symbol:         "NIFTY",
			InstrumentType: models.InstrumentTypeIndex,
			Underlying:     &underlying,
			LotSize:        1,
		},
		{
			ID:             uuid.New(),
			Symbol:         "NIFTY" + models.ContinuousFuturesSuffix,
			InstrumentType: models.InstrumentTypeFuturesIndexCont,
			Underlying:     &underlying,
			LotSize:        50,
		},
	}
}

func defaultCandles() []models.Candle {
	base := time.Date(2025, 1, 2, 9, 15, 0, 0, time.UTC)
	candles := make([]models.Candle, 5)
	for i := range candles {
		candles[i] = models.Candle{
			Timestamp: base.Add(time.Duration(i*5) * time.Minute),
			Open:      100, High: 101, Low: 99, Close: 100,
		}
	}
	return candles
}

func defaultEngineResult() *models.BacktestResult {
	return &models.BacktestResult{
		NetPnL:      150.0,
		TotalTrades: 3,
		WinCount:    2,
		LossCount:   1,
		MaxDrawdown: 0.05,
	}
}

func defaultRequest() BacktestRequest {
	return BacktestRequest{
		StrategySlug: testSlug,
		Underlying:   "NIFTY",
		From:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		To:           time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
		Lots:         2,
		Capital:      200000,
	}
}

// defaultLimits returns permissive limits suitable for unit tests that focus on
// non-limit behaviour; all checks pass and the kill switch is open.
func defaultLimits() BacktestLimits {
	return BacktestLimits{BacktestsEnabled: true, MaxDays: NoDayLimit, MaxCandles: NoCandleLimit}
}

func newService(
	br *mockBacktestRepo,
	bl *mockBuiltinLookup,
	cr *mockCandleRepo,
	ir *mockInstrumentRepo,
	eng *mockEngine,
) *BacktestService {
	svc := NewBacktestService(br, bl, cr, ir, eng, defaultLimits())
	// Run background tasks synchronously so tests can inspect final state.
	svc.launch = func(fn func()) { fn() }
	return svc
}

// -- tests --

func TestSubmit_StrategyNotFound(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{},
		emptyLookup(),
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
	)

	req := defaultRequest()
	req.StrategySlug = "nonexistent"
	_, err := svc.Submit(context.Background(), req)
	if !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}
}

func TestSubmit_InstrumentNotFound(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: []models.Instrument{}},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrNoInstrument) {
		t.Fatalf("expected ErrNoInstrument, got %v", err)
	}
}

func TestSubmit_InvalidUnderlying(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
	)

	req := defaultRequest()
	req.Underlying = "INVALID"
	_, err := svc.Submit(context.Background(), req)
	if !errors.Is(err, ErrInvalidUnderlying) {
		t.Fatalf("expected ErrInvalidUnderlying, got %v", err)
	}
}

func TestSubmit_CreateFails(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{createErr: errors.New("db error")},
		defaultLookup(),
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), defaultRequest())
	if err == nil {
		t.Fatal("expected error on create failure")
	}
}

func TestSubmit_NoCandleData(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		defaultLookup(),
		&mockCandleRepo{result: nil},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
	)

	// Candle check is synchronous — error propagates immediately, no run is created.
	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrNoCandleData) {
		t.Fatalf("expected ErrNoCandleData, got %v", err)
	}
	if br.capturedCreate != nil {
		t.Error("run should not be created when candle check fails")
	}
}

func TestSubmit_EngineError(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{err: errors.New("engine failure")},
	)

	// Submit returns RUNNING immediately; engine error is handled in the background.
	// The sync launch in tests ensures the goroutine completes before we check.
	run, err := svc.Submit(context.Background(), defaultRequest())
	if err != nil {
		t.Fatalf("unexpected error from Submit: %v", err)
	}
	if run.Status != models.BacktestRunning {
		t.Errorf("expected returned run to be RUNNING, got %s", run.Status)
	}
	if len(br.capturedUpdateResult) == 0 {
		t.Fatal("expected UpdateResult to be called on engine failure")
	}
	if br.capturedUpdateResult[0].Status != models.BacktestFailed {
		t.Errorf("expected FAILED status stored, got %s", br.capturedUpdateResult[0].Status)
	}
}

func TestSubmit_StatusTransitions(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{result: defaultEngineResult()},
	)

	run, err := svc.Submit(context.Background(), defaultRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Submit returns the run in RUNNING state immediately.
	if run.Status != models.BacktestRunning {
		t.Errorf("expected returned run to be RUNNING, got %s", run.Status)
	}

	if len(br.capturedUpdateStatus) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(br.capturedUpdateStatus))
	}
	if br.capturedUpdateStatus[0].Status != models.BacktestRunning {
		t.Errorf("expected RUNNING on UpdateStatus, got %s", br.capturedUpdateStatus[0].Status)
	}

	// Background execution (synchronous in tests) should have persisted COMPLETED.
	if len(br.capturedUpdateResult) != 1 {
		t.Fatalf("expected 1 UpdateResult call, got %d", len(br.capturedUpdateResult))
	}
	if br.capturedUpdateResult[0].Status != models.BacktestCompleted {
		t.Errorf("expected COMPLETED on UpdateResult, got %s", br.capturedUpdateResult[0].Status)
	}
}

func TestSubmit_Success_MetricsPopulated(t *testing.T) {
	br := &mockBacktestRepo{}
	engineResult := defaultEngineResult()
	svc := newService(
		br,
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{result: engineResult},
	)

	run, err := svc.Submit(context.Background(), defaultRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Returned run is RUNNING — metrics are on the persisted UpdateResult call.
	if run.InstrumentToken != "NIFTY"+models.ContinuousFuturesSuffix {
		t.Errorf("expected InstrumentToken %s, got %s", "NIFTY"+models.ContinuousFuturesSuffix, run.InstrumentToken)
	}
	if run.SignalInstrumentToken == nil || *run.SignalInstrumentToken != "NIFTY" {
		t.Errorf("expected SignalInstrumentToken %s, got %v", "NIFTY", run.SignalInstrumentToken)
	}

	if len(br.capturedUpdateResult) == 0 {
		t.Fatal("expected UpdateResult to have been called")
	}
	persisted := br.capturedUpdateResult[0]
	if persisted.NetPnl == nil || *persisted.NetPnl != engineResult.NetPnL {
		t.Errorf("expected persisted NetPnl %f, got %v", engineResult.NetPnL, persisted.NetPnl)
	}
	if persisted.TotalTrades == nil || *persisted.TotalTrades != engineResult.TotalTrades {
		t.Errorf("expected persisted TotalTrades %d, got %v", engineResult.TotalTrades, persisted.TotalTrades)
	}
	if persisted.WinCount == nil || *persisted.WinCount != engineResult.WinCount {
		t.Errorf("expected persisted WinCount %d, got %v", engineResult.WinCount, persisted.WinCount)
	}
	if persisted.LossCount == nil || *persisted.LossCount != engineResult.LossCount {
		t.Errorf("expected persisted LossCount %d, got %v", engineResult.LossCount, persisted.LossCount)
	}
	if persisted.ResultStats == nil {
		t.Error("expected ResultStats to be populated")
	}
	if persisted.ChartData == nil {
		t.Error("expected ChartData to be populated")
	}
}

func TestSubmit_FetchesSignalAndTradeCandles(t *testing.T) {
	br := &mockBacktestRepo{}
	instruments := defaultInstruments()
	signalCandles := defaultCandles()
	tradeCandles := defaultCandles()
	cr := &mockCandleRepo{
		resultByInstrument: map[uuid.UUID][]models.Candle{
			instruments[0].ID: signalCandles,
			instruments[1].ID: tradeCandles,
		},
	}
	eng := &mockEngine{result: defaultEngineResult()}
	svc := newService(
		br,
		defaultLookup(),
		cr,
		&mockInstrumentRepo{listResult: instruments},
		eng,
	)

	_, err := svc.Submit(context.Background(), defaultRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cr.queries) != 2 {
		t.Fatalf("expected 2 candle queries, got %d", len(cr.queries))
	}
	if cr.queries[0].InstrumentID != instruments[0].ID {
		t.Errorf("expected signal candle query for index instrument")
	}
	if cr.queries[1].InstrumentID != instruments[1].ID {
		t.Errorf("expected trade candle query for continuous futures instrument")
	}
	if len(eng.capturedInputs.SignalCandles) != len(signalCandles) {
		t.Errorf("expected signal candles to reach engine")
	}
	if len(eng.capturedInputs.TradeCandles) != len(tradeCandles) {
		t.Errorf("expected trade candles to reach engine")
	}
	if eng.capturedInputs.Interval != models.CandleInterval1D {
		t.Errorf("expected engine interval %q, got %q", models.CandleInterval1D, eng.capturedInputs.Interval)
	}
	if br.capturedCreate.SignalInstrumentToken == nil || *br.capturedCreate.SignalInstrumentToken != instruments[0].Symbol {
		t.Errorf("expected signal token to be persisted")
	}
	if br.capturedCreate.InstrumentToken != instruments[1].Symbol {
		t.Errorf("expected trade token to remain canonical instrument token")
	}
}

func TestSubmit_ContinuousFutureTradeCandlesCanCrossExpiryBoundary(t *testing.T) {
	instruments := defaultInstruments()
	expiryBoundary := time.Date(2025, 1, 30, 0, 0, 0, 0, time.UTC)
	signalCandles := []models.Candle{
		{Timestamp: expiryBoundary.AddDate(0, 0, -1), Open: 100, High: 101, Low: 99, Close: 100},
		{Timestamp: expiryBoundary.AddDate(0, 0, 1), Open: 101, High: 102, Low: 100, Close: 101},
	}
	tradeCandles := []models.Candle{
		{Timestamp: expiryBoundary.AddDate(0, 0, -1), Open: 200, High: 202, Low: 198, Close: 201},
		{Timestamp: expiryBoundary.AddDate(0, 0, 1), Open: 205, High: 207, Low: 203, Close: 206},
	}
	cr := &mockCandleRepo{
		resultByInstrument: map[uuid.UUID][]models.Candle{
			instruments[0].ID: signalCandles,
			instruments[1].ID: tradeCandles,
		},
	}
	eng := &mockEngine{result: defaultEngineResult()}
	svc := newService(
		&mockBacktestRepo{},
		defaultLookup(),
		cr,
		&mockInstrumentRepo{listResult: instruments},
		eng,
	)
	req := defaultRequest()
	req.From = expiryBoundary.AddDate(0, 0, -1)
	req.To = expiryBoundary.AddDate(0, 0, 1)

	_, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cr.queries) != 2 {
		t.Fatalf("expected 2 candle queries, got %d", len(cr.queries))
	}
	if cr.queries[1].InstrumentID != instruments[1].ID {
		t.Errorf("expected continuous futures query for trade candles")
	}
	if len(eng.capturedInputs.TradeCandles) != len(tradeCandles) {
		t.Errorf("expected trade candles across expiry boundary to reach engine")
	}
}

func TestSubmit_SameInstrumentSourcesPersistSameToken(t *testing.T) {
	underlying := "RELIANCE"
	builtin := defaultBuiltin()
	builtin.InstrumentType = models.InstrumentTypeEquity
	builtin.SourceResolver = fixedTestSourceResolver{
		sources: models.StrategySources{
			Signal: models.InstrumentSpec{Kind: models.InstrumentKindEquity},
			Trade:  models.InstrumentSpec{Kind: models.InstrumentKindEquity},
		},
	}
	builtin.Inputs[0].Options = []string{underlying}
	lookup := &mockBuiltinLookup{
		strategies: map[string]*models.BuiltinStrategy{testSlug: builtin},
		order:      []string{testSlug},
	}
	instrument := models.Instrument{
		ID:             uuid.New(),
		Symbol:         underlying,
		InstrumentType: models.InstrumentTypeEquity,
		Underlying:     &underlying,
		LotSize:        1,
	}
	cr := &mockCandleRepo{
		resultByInstrument: map[uuid.UUID][]models.Candle{
			instrument.ID: defaultCandles(),
		},
	}
	svc := newService(
		&mockBacktestRepo{},
		lookup,
		cr,
		&mockInstrumentRepo{listResult: []models.Instrument{instrument}},
		&mockEngine{result: defaultEngineResult()},
	)
	req := defaultRequest()
	req.Underlying = underlying

	run, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.InstrumentToken != underlying {
		t.Errorf("expected trade token %q, got %q", underlying, run.InstrumentToken)
	}
	if run.SignalInstrumentToken == nil || *run.SignalInstrumentToken != underlying {
		t.Errorf("expected signal token %q, got %v", underlying, run.SignalInstrumentToken)
	}
}

func TestSubmit_InvalidSourceInterval(t *testing.T) {
	builtin := defaultBuiltin()
	builtin.CandleInterval = unsupportedStrategyInterval
	lookup := &mockBuiltinLookup{
		strategies: map[string]*models.BuiltinStrategy{testSlug: builtin},
		order:      []string{testSlug},
	}
	svc := newService(
		&mockBacktestRepo{},
		lookup,
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrInvalidStrategySources) {
		t.Fatalf("expected ErrInvalidStrategySources, got %v", err)
	}
}

func TestSubmit_AmbiguousInstrumentResolution(t *testing.T) {
	underlying := models.UnderlyingNifty
	duplicateIndex := models.Instrument{
		ID:             uuid.New(),
		Symbol:         "NIFTY_ALT",
		InstrumentType: models.InstrumentTypeIndex,
		Underlying:     &underlying,
		LotSize:        1,
	}
	instruments := append(defaultInstruments(), duplicateIndex)
	svc := newService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: instruments},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrInvalidStrategySources) {
		t.Fatalf("expected ErrInvalidStrategySources, got %v", err)
	}
}

func TestSubmit_RejectsNonContinuousFuturesSourceKindInV1(t *testing.T) {
	builtin := defaultBuiltin()
	builtin.SourceResolver = fixedTestSourceResolver{
		sources: models.StrategySources{
			Signal: models.InstrumentSpec{Kind: models.InstrumentKindIndex},
			Trade:  models.InstrumentSpec{Kind: models.InstrumentKindFuturesIndex},
		},
	}
	lookup := &mockBuiltinLookup{
		strategies: map[string]*models.BuiltinStrategy{testSlug: builtin},
		order:      []string{testSlug},
	}
	svc := newService(
		&mockBacktestRepo{},
		lookup,
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrInvalidStrategySources) {
		t.Fatalf("expected ErrInvalidStrategySources, got %v", err)
	}
}

func TestSubmit_RunCarriesSlugAndInputs(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{result: defaultEngineResult()},
	)

	req := defaultRequest()
	run, err := svc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.StrategySlug == nil || *run.StrategySlug != req.StrategySlug {
		t.Errorf("expected StrategySlug %q, got %v", req.StrategySlug, run.StrategySlug)
	}
	if run.Capital == nil || *run.Capital != req.Capital {
		t.Errorf("expected Capital %f, got %v", req.Capital, run.Capital)
	}
	if run.Lots == nil || *run.Lots != req.Lots {
		t.Errorf("expected Lots %d, got %v", req.Lots, run.Lots)
	}
	if run.Underlying == nil || *run.Underlying != req.Underlying {
		t.Errorf("expected Underlying %q, got %v", req.Underlying, run.Underlying)
	}
	if run.StrategyID != nil {
		t.Errorf("expected StrategyID to be nil for built-in, got %v", run.StrategyID)
	}
}

func TestSubmit_FailRunStoresSafeMessage(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{err: errors.New("pq: connection refused to 10.0.0.5:5432")},
	)

	_, _ = svc.Submit(context.Background(), defaultRequest())
	if len(br.capturedUpdateResult) == 0 {
		t.Fatal("expected UpdateResult to be called on failure")
	}
	stored := br.capturedUpdateResult[0]
	if stored.ErrorMessage == nil {
		t.Fatal("expected ErrorMessage to be set")
	}
	if *stored.ErrorMessage != errMsgInternalError {
		t.Errorf("expected safe message %q, got %q", errMsgInternalError, *stored.ErrorMessage)
	}
}

func TestGetByID(t *testing.T) {
	id := uuid.New()
	expected := &models.BacktestRun{ID: id, Status: models.BacktestCompleted}
	br := &mockBacktestRepo{getByIDResult: expected}
	svc := newService(br, emptyLookup(), &mockCandleRepo{}, &mockInstrumentRepo{}, &mockEngine{})

	run, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.ID != id {
		t.Errorf("expected ID %s, got %s", id, run.ID)
	}
}

func TestListCompleted_PopulatesStrategyNamesAndPassesPagination(t *testing.T) {
	slug := testSlug
	runs := []models.BacktestRun{
		{ID: uuid.New(), Status: models.BacktestCompleted, StrategySlug: &slug},
	}
	br := &mockBacktestRepo{listResult: runs, listTotal: 8}
	svc := newService(br, defaultLookup(), &mockCandleRepo{}, &mockInstrumentRepo{}, &mockEngine{})

	result, total, err := svc.ListCompleted(context.Background(), 2, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 8 {
		t.Errorf("expected total 8, got %d", total)
	}
	if br.capturedListPage != 2 || br.capturedListLimit != 20 {
		t.Errorf("expected pagination 2/20, got %d/%d", br.capturedListPage, br.capturedListLimit)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 run, got %d", len(result))
	}
	if result[0].StrategyName == nil || *result[0].StrategyName != defaultBuiltin().Name {
		t.Fatalf("expected strategy name %q, got %v", defaultBuiltin().Name, result[0].StrategyName)
	}
}

func TestSubmit_KillSwitchDisabled(t *testing.T) {
	svc := NewBacktestService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
		BacktestLimits{BacktestsEnabled: false},
	)
	svc.launch = func(fn func()) { fn() }

	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrBacktestDisabled) {
		t.Fatalf("expected ErrBacktestDisabled, got %v", err)
	}
}

func TestSubmit_DateRangeExceeded(t *testing.T) {
	svc := NewBacktestService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
		BacktestLimits{BacktestsEnabled: true, MaxDays: 30},
	)
	svc.launch = func(fn func()) { fn() }

	req := defaultRequest()
	// defaultRequest spans Jan 1 – Jun 30 = 181 inclusive days, exceeds MaxDays=30.
	_, err := svc.Submit(context.Background(), req)
	if !errors.Is(err, ErrBacktestDateRangeExceeded) {
		t.Fatalf("expected ErrBacktestDateRangeExceeded, got %v", err)
	}
}

func TestSubmit_DateRangeAtExactLimit_Passes(t *testing.T) {
	svc := NewBacktestService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{result: defaultEngineResult()},
		BacktestLimits{BacktestsEnabled: true, MaxDays: 181},
	)
	svc.launch = func(fn func()) { fn() }

	req := defaultRequest()
	// defaultRequest spans Jan 1 – Jun 30 = 181 inclusive days; MaxDays=181 must pass.
	_, err := svc.Submit(context.Background(), req)
	if errors.Is(err, ErrBacktestDateRangeExceeded) {
		t.Fatal("expected request with exactly MaxDays to pass, but got ErrBacktestDateRangeExceeded")
	}
}

func TestSubmit_CandleCountExceeded(t *testing.T) {
	candles := defaultCandles() // 5 candles

	svc := NewBacktestService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{result: candles},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{},
		BacktestLimits{BacktestsEnabled: true, MaxCandles: 3},
	)
	svc.launch = func(fn func()) { fn() }

	_, err := svc.Submit(context.Background(), defaultRequest())
	if !errors.Is(err, ErrBacktestCandleCountExceeded) {
		t.Fatalf("expected ErrBacktestCandleCountExceeded, got %v", err)
	}
}

func TestSubmit_CandleCountAtExactLimit_Passes(t *testing.T) {
	candles := defaultCandles() // 5 candles

	svc := NewBacktestService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{result: candles},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{result: defaultEngineResult()},
		BacktestLimits{BacktestsEnabled: true, MaxCandles: 5},
	)
	svc.launch = func(fn func()) { fn() }

	_, err := svc.Submit(context.Background(), defaultRequest())
	if errors.Is(err, ErrBacktestCandleCountExceeded) {
		t.Fatal("expected request with exactly MaxCandles to pass, but got ErrBacktestCandleCountExceeded")
	}
}

func TestSubmit_NoDayLimit_AllowsLargeRange(t *testing.T) {
	svc := NewBacktestService(
		&mockBacktestRepo{},
		defaultLookup(),
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{listResult: defaultInstruments()},
		&mockEngine{result: defaultEngineResult()},
		BacktestLimits{BacktestsEnabled: true, MaxDays: NoDayLimit, MaxCandles: NoCandleLimit},
	)
	svc.launch = func(fn func()) { fn() }

	req := defaultRequest()
	req.From = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	req.To = time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	_, err := svc.Submit(context.Background(), req)
	if errors.Is(err, ErrBacktestDateRangeExceeded) {
		t.Fatal("expected no date-range check when MaxDays=0, but got ErrBacktestDateRangeExceeded")
	}
}
