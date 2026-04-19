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
	createErr      error
	updateStatusErr error
	updateResultErr error
	getByIDResult  *models.BacktestRun
	getByIDErr     error
	listResult     []models.BacktestRun
	listErr        error

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

type mockStrategyRepo struct {
	result *models.Strategy
	err    error
}

func (m *mockStrategyRepo) GetByID(_ context.Context, _ uuid.UUID) (*models.Strategy, error) {
	return m.result, m.err
}
func (m *mockStrategyRepo) List(_ context.Context) ([]models.Strategy, error)         { return nil, nil }
func (m *mockStrategyRepo) Create(_ context.Context, _ *models.Strategy) error        { return nil }
func (m *mockStrategyRepo) Update(_ context.Context, _ *models.Strategy) error        { return nil }
func (m *mockStrategyRepo) Delete(_ context.Context, _ uuid.UUID) error               { return nil }

type mockCandleRepo struct {
	result []models.Candle
	err    error
}

func (m *mockCandleRepo) Query(_ context.Context, _ models.CandleFilter) ([]models.Candle, error) {
	return m.result, m.err
}

type mockInstrumentRepo struct {
	result *models.Instrument
	err    error
}

func (m *mockInstrumentRepo) GetByID(_ context.Context, _ uuid.UUID) (*models.Instrument, error) {
	return m.result, m.err
}
func (m *mockInstrumentRepo) List(_ context.Context, _ models.InstrumentFilter) ([]models.Instrument, error) {
	return nil, nil
}

type mockEngine struct {
	result *models.BacktestResult
	err    error
}

func (m *mockEngine) RunBacktest(_ *models.Strategy, _ []models.Candle) (*models.BacktestResult, error) {
	return m.result, m.err
}

// -- helpers --

func defaultStrategy() *models.Strategy {
	return &models.Strategy{
		ID:                 uuid.New(),
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
	}
}

func defaultInstrument() *models.Instrument {
	return &models.Instrument{ID: uuid.New(), Symbol: "NIFTY"}
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
	netPnL := 150.0
	total := 3
	wins := 2
	losses := 1
	dd := 0.05
	_ = netPnL
	return &models.BacktestResult{
		NetPnL:      150.0,
		TotalTrades: total,
		WinCount:    wins,
		LossCount:   losses,
		MaxDrawdown: dd,
	}
}

func newService(
	br *mockBacktestRepo,
	sr *mockStrategyRepo,
	cr *mockCandleRepo,
	ir *mockInstrumentRepo,
	eng *mockEngine,
) *BacktestService {
	return NewBacktestService(br, sr, cr, ir, eng)
}

// -- tests --

func TestSubmit_StrategyNotFound(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{},
		&mockStrategyRepo{err: errors.New("not found")},
		&mockCandleRepo{},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID:   uuid.New(),
		InstrumentID: uuid.New(),
	})
	if err == nil || err.Error() != "strategy not found" {
		t.Fatalf("expected strategy not found, got %v", err)
	}
}

func TestSubmit_InstrumentNotFound(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{},
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{},
		&mockInstrumentRepo{err: errors.New("not found")},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID:   uuid.New(),
		InstrumentID: uuid.New(),
	})
	if err == nil || err.Error() != "instrument not found" {
		t.Fatalf("expected instrument not found, got %v", err)
	}
}

func TestSubmit_CreateFails(t *testing.T) {
	svc := newService(
		&mockBacktestRepo{createErr: errors.New("db error")},
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID: uuid.New(), InstrumentID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error on create failure")
	}
}

func TestSubmit_NoCandleData(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{result: nil},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{},
	)

	_, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID: uuid.New(), InstrumentID: uuid.New(),
	})
	if err == nil || err.Error() != "no candle data available" {
		t.Fatalf("expected no candle data available, got %v", err)
	}
	// run should be marked FAILED
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
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{err: errors.New("engine failure")},
	)

	_, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID: uuid.New(), InstrumentID: uuid.New(),
	})
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
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{result: defaultEngineResult()},
	)

	run, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID: uuid.New(), InstrumentID: uuid.New(),
		From: time.Now(), To: time.Now(), Interval: "5m",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UpdateStatus called once with RUNNING
	if len(br.capturedUpdateStatus) != 1 {
		t.Fatalf("expected 1 UpdateStatus call, got %d", len(br.capturedUpdateStatus))
	}
	if br.capturedUpdateStatus[0].Status != models.BacktestRunning {
		t.Errorf("expected RUNNING on UpdateStatus, got %s", br.capturedUpdateStatus[0].Status)
	}

	// UpdateResult called once with COMPLETED
	if len(br.capturedUpdateResult) != 1 {
		t.Fatalf("expected 1 UpdateResult call, got %d", len(br.capturedUpdateResult))
	}
	if br.capturedUpdateResult[0].Status != models.BacktestCompleted {
		t.Errorf("expected COMPLETED on UpdateResult, got %s", br.capturedUpdateResult[0].Status)
	}

	// Returned run is COMPLETED
	if run.Status != models.BacktestCompleted {
		t.Errorf("expected returned run to be COMPLETED, got %s", run.Status)
	}
}

func TestSubmit_Success_MetricsPopulated(t *testing.T) {
	br := &mockBacktestRepo{}
	engineResult := defaultEngineResult()
	svc := newService(
		br,
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{result: engineResult},
	)

	run, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID: uuid.New(), InstrumentID: uuid.New(),
		From: time.Now(), To: time.Now(), Interval: "5m",
	})
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

func TestSubmit_UpdateStatusNotCalledWithCompletedAt(t *testing.T) {
	br := &mockBacktestRepo{}
	svc := newService(
		br,
		&mockStrategyRepo{result: defaultStrategy()},
		&mockCandleRepo{result: defaultCandles()},
		&mockInstrumentRepo{result: defaultInstrument()},
		&mockEngine{result: defaultEngineResult()},
	)

	_, err := svc.Submit(context.Background(), BacktestRequest{
		StrategyID: uuid.New(), InstrumentID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UpdateStatus must only be called for RUNNING — never COMPLETED/FAILED
	for _, run := range br.capturedUpdateStatus {
		if run.Status != models.BacktestRunning {
			t.Errorf("UpdateStatus called with non-RUNNING status: %s", run.Status)
		}
	}
}

func TestGetByID(t *testing.T) {
	id := uuid.New()
	expected := &models.BacktestRun{ID: id, Status: models.BacktestCompleted}
	br := &mockBacktestRepo{getByIDResult: expected}
	svc := newService(br, &mockStrategyRepo{}, &mockCandleRepo{}, &mockInstrumentRepo{}, &mockEngine{})

	run, err := svc.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.ID != id {
		t.Errorf("expected ID %s, got %s", id, run.ID)
	}
}

func TestListByStrategy(t *testing.T) {
	strategyID := uuid.New()
	expected := []models.BacktestRun{
		{ID: uuid.New(), Status: models.BacktestCompleted},
		{ID: uuid.New(), Status: models.BacktestFailed},
	}
	br := &mockBacktestRepo{listResult: expected}
	svc := newService(br, &mockStrategyRepo{}, &mockCandleRepo{}, &mockInstrumentRepo{}, &mockEngine{})

	runs, err := svc.ListByStrategy(context.Background(), strategyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
}
