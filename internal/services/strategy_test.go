package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// -- strategy-specific mocks --

type mockBacktestRepoForStrategy struct {
	latestBySlug map[string]*models.BacktestRun
}

func (m *mockBacktestRepoForStrategy) Create(_ context.Context, _ *models.BacktestRun) error {
	return nil
}
func (m *mockBacktestRepoForStrategy) UpdateStatus(_ context.Context, _ *models.BacktestRun) error {
	return nil
}
func (m *mockBacktestRepoForStrategy) UpdateResult(_ context.Context, _ *models.BacktestRun) error {
	return nil
}
func (m *mockBacktestRepoForStrategy) GetByID(_ context.Context, _ uuid.UUID) (*models.BacktestRun, error) {
	return nil, models.ErrNotFound
}
func (m *mockBacktestRepoForStrategy) ListByStrategy(_ context.Context, _ uuid.UUID) ([]models.BacktestRun, error) {
	return nil, nil
}
func (m *mockBacktestRepoForStrategy) LatestCompletedBySlug(_ context.Context, slug string) (*models.BacktestRun, error) {
	if run, ok := m.latestBySlug[slug]; ok {
		return run, nil
	}
	return nil, models.ErrNotFound
}
func (m *mockBacktestRepoForStrategy) GetByIDWithTrades(_ context.Context, _ uuid.UUID) (*models.BacktestRun, error) {
	return nil, models.ErrNotFound
}
func (m *mockBacktestRepoForStrategy) ListCompleted(_ context.Context, _, _ int) ([]models.BacktestRun, int, error) {
	return nil, 0, nil
}

type mockCandleRepoForStrategy struct {
	maxDate time.Time
	err     error
}

func (m *mockCandleRepoForStrategy) Query(_ context.Context, _ models.CandleFilter) ([]models.Candle, error) {
	return nil, nil
}
func (m *mockCandleRepoForStrategy) InsertBatchIgnoreDuplicates(_ context.Context, _ []models.Candle) (int64, error) {
	return 0, nil
}
func (m *mockCandleRepoForStrategy) MaxDate(_ context.Context) (time.Time, error) {
	return m.maxDate, m.err
}

func newStrategySvc(bl *mockBuiltinLookup, br *mockBacktestRepoForStrategy, cr *mockCandleRepoForStrategy) *StrategyService {
	return NewStrategyService(bl, br, cr)
}

// -- tests --

func TestListSections_ReturnsBothSections(t *testing.T) {
	svc := newStrategySvc(
		defaultLookup(),
		&mockBacktestRepoForStrategy{latestBySlug: map[string]*models.BacktestRun{}},
		&mockCandleRepoForStrategy{},
	)

	sections, err := svc.ListSections(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Key != SectionKeyBuiltin {
		t.Errorf("expected first section key %s, got %s", SectionKeyBuiltin, sections[0].Key)
	}
	if sections[1].Key != SectionKeyCustom {
		t.Errorf("expected second section key %s, got %s", SectionKeyCustom, sections[1].Key)
	}
}

func TestListSections_BuiltinHasStrategy(t *testing.T) {
	svc := newStrategySvc(
		defaultLookup(),
		&mockBacktestRepoForStrategy{latestBySlug: map[string]*models.BacktestRun{}},
		&mockCandleRepoForStrategy{},
	)

	sections, err := svc.ListSections(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sections[0].Strategies) != 1 {
		t.Fatalf("expected 1 built-in strategy, got %d", len(sections[0].Strategies))
	}
	if sections[0].Strategies[0].Strategy.ID != testSlug {
		t.Errorf("expected slug %q, got %q", testSlug, sections[0].Strategies[0].Strategy.ID)
	}
	if sections[0].Strategies[0].LastBacktest != nil {
		t.Error("expected nil LastBacktest when none exists")
	}
}

func TestListSections_BuiltinWithLastBacktest(t *testing.T) {
	runID := uuid.New()
	svc := newStrategySvc(
		defaultLookup(),
		&mockBacktestRepoForStrategy{
			latestBySlug: map[string]*models.BacktestRun{
				testSlug: {ID: runID, Status: models.BacktestCompleted},
			},
		},
		&mockCandleRepoForStrategy{},
	)

	sections, err := svc.ListSections(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := sections[0].Strategies[0]
	if item.LastBacktest == nil {
		t.Fatal("expected LastBacktest to be populated")
	}
	if item.LastBacktest.ID != runID {
		t.Errorf("expected backtest ID %s, got %s", runID, item.LastBacktest.ID)
	}
}

func TestListSections_CustomIsEmpty(t *testing.T) {
	svc := newStrategySvc(
		defaultLookup(),
		&mockBacktestRepoForStrategy{latestBySlug: map[string]*models.BacktestRun{}},
		&mockCandleRepoForStrategy{},
	)

	sections, err := svc.ListSections(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	custom := sections[1]
	if len(custom.Strategies) != 0 {
		t.Errorf("expected 0 custom strategies, got %d", len(custom.Strategies))
	}
}

func TestGetBySlug_Found(t *testing.T) {
	maxDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	svc := newStrategySvc(
		defaultLookup(),
		&mockBacktestRepoForStrategy{latestBySlug: map[string]*models.BacktestRun{}},
		&mockCandleRepoForStrategy{maxDate: maxDate},
	)

	detail, err := svc.GetBySlug(context.Background(), testSlug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Strategy.ID != testSlug {
		t.Errorf("expected slug %q, got %q", testSlug, detail.Strategy.ID)
	}
	if !detail.MaxDate.Equal(maxDate) {
		t.Errorf("expected maxDate %v, got %v", maxDate, detail.MaxDate)
	}
	if detail.LastBacktest != nil {
		t.Error("expected nil LastBacktest when none exists")
	}
}

func TestGetBySlug_NotFound(t *testing.T) {
	svc := newStrategySvc(
		emptyLookup(),
		&mockBacktestRepoForStrategy{latestBySlug: map[string]*models.BacktestRun{}},
		&mockCandleRepoForStrategy{},
	)

	_, err := svc.GetBySlug(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent slug")
	}
}
