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

	capturedCreate       *models.BacktestRun
	capturedUpdateStatus []*models.BacktestRun
	capturedUpdateResult []*models.BacktestRun
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
	result []models.Candle
	err    error
}

func (m *mockCandleRepo) Query(_ context.Context, _ models.CandleFilter) ([]models.Candle, error) {
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
func (m *mockInstrumentRepo) List(_ context.Context, _ models.InstrumentFilter) ([]models.Instrument, error) {
	return m.listResult, m.listErr
}
func (m *mockInstrumentRepo) UpsertBatch(_ context.Context, _ []models.Instrument) error {
	return nil
}

type mockEngine struct {
	result *models.BacktestResult
	err    error
}

func (m *mockEngine) RunBacktest(_ *models.Strategy, _ []models.Candle) (*models.BacktestResult, error) {
	return m.result, m.err
}

// -- helpers --

const testSlug = "ma_crossover"

func defaultBuiltin() *models.BuiltinStrategy {
	return &models.BuiltinStrategy{
		ID:                 testSlug,
		Name:               "MA Crossover",
		EntryConditionType: models.EntryConditionMACrossover,
		InstrumentType:     models.InstrumentTypeFuturesIndex,
		ExpiryRule:         models.ExpiryRuleCurrentMonth,
		CandleInterval:     models.CandleInterval1D,
	}
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
	return []models.Instrument{
		{ID: uuid.New(), Symbol: "NIFTY", LotSize: 50},
	}
}

func defaultCandles() []models.Candle {
	base := time.Date(2025, 1, 2, 9, 15, 0, 0, time.UTC)
	candles := make([]models.Candle, 5)
	for i := range candles {
		candles[i] = models.Candle{
			Timestamp: base.Add(time.Duration(i*5) * time.Minute),
			Open: 100, High: 101, Low: 99, Close: 100,
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

func newService(
	br *mockBacktestRepo,
	bl *mockBuiltinLookup,
	cr *mockCandleRepo,
	ir *mockInstrumentRepo,
	eng *mockEngine,
) *BacktestService {
	return NewBacktestService(br, bl, cr, ir, eng)
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
	if err == nil || err.Error() != "strategy not found" {
		t.Fatalf("expected strategy not found, got %v", err)
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
	if err == nil || err.Error() != "no instrument found for underlying" {
		t.Fatalf("expected no instrument found, got %v", err)
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

	_, err := svc.Submit(context.Background(), defaultRequest())
	if err == nil || err.Error() != "no candle data available" {
		t.Fatalf("expected no candle data available, got %v", err)
	}
	if len(br.capturedUpdateResult) == 0 {
		t.Fatal("expected UpdateResult to be called on failure")
	}
	if br.capturedUpdateResult[0].Status != models.BacktestFailed {
		t.Errorf("expected FAILED status, got %s", br.capturedUpdateResult[0].Status)
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

	_, err := svc.Submit(context.Background(), defaultRequest())
	if err == nil {
		t.Fatal("expected error from engine failure")
	}
	if len(br.capturedUpdateResult) == 0 {
		t.Fatal("expected UpdateResult to be called on engine failure")
	}
	if br.capturedUpdateResult[0].Status != models.BacktestFailed {
		t.Errorf("expected FAILED status, got %s", br.capturedUpdateResult[0].Status)
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

	if len(br.capturedUpdateStatus) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(br.capturedUpdateStatus))
	}
	if br.capturedUpdateStatus[0].Status != models.BacktestRunning {
		t.Errorf("expected RUNNING on UpdateStatus, got %s", br.capturedUpdateStatus[0].Status)
	}

	if len(br.capturedUpdateResult) != 1 {
		t.Fatalf("expected 1 UpdateResult call, got %d", len(br.capturedUpdateResult))
	}
	if br.capturedUpdateResult[0].Status != models.BacktestCompleted {
		t.Errorf("expected COMPLETED on UpdateResult, got %s", br.capturedUpdateResult[0].Status)
	}

	if run.Status != models.BacktestCompleted {
		t.Errorf("expected returned run to be COMPLETED, got %s", run.Status)
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

	if run.NetPnl == nil || *run.NetPnl != engineResult.NetPnL {
		t.Errorf("expected NetPnl %f, got %v", engineResult.NetPnL, run.NetPnl)
	}
	if run.TotalTrades == nil || *run.TotalTrades != engineResult.TotalTrades {
		t.Errorf("expected TotalTrades %d, got %v", engineResult.TotalTrades, run.TotalTrades)
	}
	if run.WinCount == nil || *run.WinCount != engineResult.WinCount {
		t.Errorf("expected WinCount %d, got %v", engineResult.WinCount, run.WinCount)
	}
	if run.LossCount == nil || *run.LossCount != engineResult.LossCount {
		t.Errorf("expected LossCount %d, got %v", engineResult.LossCount, run.LossCount)
	}
	if run.InstrumentToken != "NIFTY" {
		t.Errorf("expected InstrumentToken NIFTY, got %s", run.InstrumentToken)
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
