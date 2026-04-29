package handlers

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
)

const floatTolerance = 1e-9

func TestStrategyListItemResponseJSONShape(t *testing.T) {
	resp := strategyListItemResponse{
		ID:          "ma_crossover",
		Name:        "MA Crossover",
		Category:    "Trend",
		Description: "desc",
	}
	keys := jsonKeys(t, resp)
	want := []string{"category", "description", "id", "lastBacktest", "name"}
	assertKeysEqual(t, keys, want)

	forbidden := []string{"strategy_id", "entry_condition_type", "instrument_type", "expiry_rule", "candle_interval"}
	for _, f := range forbidden {
		for _, k := range keys {
			if k == f {
				t.Fatalf("forbidden key %q appeared in strategyListItemResponse JSON", f)
			}
		}
	}
}

func TestStrategyDetailResponseJSONShape(t *testing.T) {
	resp := strategyDetailResponse{
		ID:          "ma_crossover",
		Name:        "MA Crossover",
		Category:    "Trend",
		Description: "desc",
		Logic:       []string{"line1"},
		Inputs:      []strategyInputResponse{},
	}
	keys := jsonKeys(t, resp)
	want := []string{"category", "description", "id", "inputs", "lastBacktest", "logic", "name"}
	assertKeysEqual(t, keys, want)
}

func TestStrategySectionResponseJSONShape(t *testing.T) {
	resp := strategySectionResponse{
		Key:        "BUILTIN",
		Label:      "Strategies",
		Strategies: []strategyListItemResponse{},
	}
	keys := jsonKeys(t, resp)
	want := []string{"key", "label", "placeholder", "strategies"}
	assertKeysEqual(t, keys, want)
}

func TestLastBacktestSummaryResponseJSONShape(t *testing.T) {
	resp := lastBacktestSummaryResponse{
		ID:         "abc",
		ReturnPct:  11.2,
		TradeCount: 22,
		RanAt:      "2025-04-20T10:30:00Z",
	}
	keys := jsonKeys(t, resp)
	want := []string{"id", "ranAt", "returnPct", "tradeCount"}
	assertKeysEqual(t, keys, want)
}

func TestLastBacktestDetailResponseJSONShape(t *testing.T) {
	resp := lastBacktestDetailResponse{
		ID:         "abc",
		ReturnPct:  11.2,
		WinRate:    49,
		TradeCount: 22,
		RanAt:      "2025-04-20T10:30:00Z",
	}
	keys := jsonKeys(t, resp)
	want := []string{"id", "ranAt", "returnPct", "tradeCount", "winRate"}
	assertKeysEqual(t, keys, want)
}

func TestComputeReturnPct(t *testing.T) {
	capital := 200000.0
	pnl := 22400.0
	run := &models.BacktestRun{Capital: &capital, NetPnl: &pnl}
	got := computeReturnPct(run)
	want := 11.2
	if math.Abs(got-want) > floatTolerance {
		t.Errorf("returnPct = %f, want %f", got, want)
	}
}

func TestComputeReturnPct_NilCapital(t *testing.T) {
	pnl := 100.0
	run := &models.BacktestRun{NetPnl: &pnl}
	if got := computeReturnPct(run); got != 0 {
		t.Errorf("expected 0 when capital is nil, got %f", got)
	}
}

func TestComputeWinRate(t *testing.T) {
	total := 22
	wins := 11
	run := &models.BacktestRun{TotalTrades: &total, WinCount: &wins}
	got := computeWinRate(run)
	if got != 50 {
		t.Errorf("winRate = %d, want 50", got)
	}
}

func TestComputeWinRate_ZeroTrades(t *testing.T) {
	total := 0
	wins := 0
	run := &models.BacktestRun{TotalTrades: &total, WinCount: &wins}
	if got := computeWinRate(run); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestToStrategyInputResponses_InjectsMaxDate(t *testing.T) {
	inputs := []models.StrategyInput{
		{
			Key:         "dateRange",
			Label:       "Date range",
			Type:        models.InputTypeDateRange,
			Constraints: map[string]any{models.ConstraintMinDate: "2022-01-01"},
			DefaultFrom: "2025-01-01",
			DefaultTo:   "2025-12-31",
		},
		{
			Key:   "lots",
			Label: "Lots",
			Type:  models.InputTypeNumber,
		},
	}
	maxDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	result := toStrategyInputResponses(inputs, maxDate)

	dateInput := result[0]
	if dateInput.Constraints[constraintMaxDate] != "2025-06-15" {
		t.Errorf("expected maxDate constraint '2025-06-15', got %v", dateInput.Constraints[constraintMaxDate])
	}
	if dateInput.Constraints[models.ConstraintMinDate] != "2022-01-01" {
		t.Error("original minDate constraint was lost")
	}

	lotsInput := result[1]
	if _, ok := lotsInput.Constraints[constraintMaxDate]; ok {
		t.Error("maxDate should not be injected into non-DATE_RANGE inputs")
	}
}

func TestToStrategyInputResponses_DoesNotMutateOriginal(t *testing.T) {
	original := map[string]any{models.ConstraintMinDate: "2022-01-01"}
	inputs := []models.StrategyInput{
		{Key: "dateRange", Type: models.InputTypeDateRange, Constraints: original},
	}
	maxDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	toStrategyInputResponses(inputs, maxDate)

	if _, ok := original[constraintMaxDate]; ok {
		t.Error("original constraints map was mutated")
	}
}

func TestToStrategySectionsResponse_MatchesContract(t *testing.T) {
	sections := []services.StrategySection{
		{
			Key: "BUILTIN",
			Strategies: []services.StrategyListItem{
				{
					Strategy: &models.BuiltinStrategy{
						ID: "ma_crossover", Name: "MA Crossover",
						Category: "Trend", Description: "desc",
					},
				},
			},
		},
		{
			Key:        "CUSTOM",
			Strategies: []services.StrategyListItem{},
		},
	}

	resp := toStrategySectionsResponse(sections)
	if len(resp.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(resp.Sections))
	}
	builtin := resp.Sections[0]
	if builtin.Key != "BUILTIN" {
		t.Errorf("expected BUILTIN, got %s", builtin.Key)
	}
	if builtin.Label != sectionLabelBuiltin {
		t.Errorf("expected label %q, got %q", sectionLabelBuiltin, builtin.Label)
	}
	if len(builtin.Strategies) != 1 {
		t.Errorf("expected 1 builtin strategy, got %d", len(builtin.Strategies))
	}
	custom := resp.Sections[1]
	if custom.Label != sectionLabelCustom {
		t.Errorf("expected label %q, got %q", sectionLabelCustom, custom.Label)
	}
	if custom.Placeholder == nil {
		t.Error("CUSTOM section should have placeholder")
	}
	if custom.Placeholder.Title != placeholderTitleCustom {
		t.Errorf("expected placeholder title %q, got %q", placeholderTitleCustom, custom.Placeholder.Title)
	}
}

func TestToLastBacktestSummary_WithRun(t *testing.T) {
	id := uuid.New()
	completedAt := time.Date(2025, 4, 20, 10, 30, 0, 0, time.UTC)
	capital := 200000.0
	pnl := 22400.0
	total := 22
	run := &models.BacktestRun{
		ID:          id,
		NetPnl:      &pnl,
		TotalTrades: &total,
		Capital:     &capital,
		CompletedAt: &completedAt,
	}
	resp := toLastBacktestSummary(run)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.ID != id.String() {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}
	if math.Abs(resp.ReturnPct-11.2) > floatTolerance {
		t.Errorf("expected returnPct 11.2, got %f", resp.ReturnPct)
	}
	if resp.TradeCount != 22 {
		t.Errorf("expected tradeCount 22, got %d", resp.TradeCount)
	}
	if resp.RanAt != "2025-04-20T10:30:00Z" {
		t.Errorf("expected ranAt 2025-04-20T10:30:00Z, got %s", resp.RanAt)
	}
}

func TestToLastBacktestSummary_Nil(t *testing.T) {
	if resp := toLastBacktestSummary(nil); resp != nil {
		t.Error("expected nil for nil run")
	}
}
