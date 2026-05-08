package handlers

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func TestBacktestSubmitResponseJSONShape(t *testing.T) {
	resp := backtestSubmitResponse{ID: "abc", Status: "RUNNING"}
	keys := jsonKeys(t, resp)
	want := []string{"id", "status"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestStatusResponseJSONShape_Running(t *testing.T) {
	resp := backtestStatusResponse{ID: "abc", Status: "RUNNING"}
	keys := jsonKeys(t, resp)
	// errorMessage and result are omitempty — absent when nil
	want := []string{"id", "status"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestStatusResponseJSONShape_Completed(t *testing.T) {
	resp := backtestStatusResponse{
		ID:     "abc",
		Status: "COMPLETED",
		Result: &backtestResultPayload{
			Chart: backtestChartResponse{
				Equity:   []chartPointResponse{},
				Drawdown: []chartPointResponse{},
			},
		},
	}
	keys := jsonKeys(t, resp)
	want := []string{"id", "result", "status"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestStatusResponseJSONShape_Failed(t *testing.T) {
	msg := "no candle data available"
	resp := backtestStatusResponse{ID: "abc", Status: "FAILED", ErrorMessage: &msg}
	keys := jsonKeys(t, resp)
	want := []string{"errorMessage", "id", "status"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestResultPayloadJSONShape(t *testing.T) {
	resp := backtestResultPayload{
		Chart: backtestChartResponse{
			Equity:   []chartPointResponse{},
			Drawdown: []chartPointResponse{},
		},
	}
	keys := jsonKeys(t, resp)
	want := []string{
		"avgHoldingMinutes", "avgLoss", "avgPnlPerTrade", "avgWin",
		"bestTrade", "capEnd", "capStart", "chart",
		"from", "interval", "longestLossStreak", "longestWinStreak",
		"lots", "maxDrawdownPct", "netPnl", "profitFactor", "returnPct",
		"rewardRisk", "strategy", "to",
		"tradeCount", "tradesPerWeek", "underlying", "winRate", "worstTrade",
	}
	assertKeysEqual(t, keys, want)
}

func TestBacktestTradeResponseJSONShape(t *testing.T) {
	resp := backtestTradeResponse{
		EntryTs: "2025-01-02T09:15:00Z", ExitTs: "2025-01-02T09:30:00Z",
		Side: "BUY", Quantity: 50, EntryPrice: 100.0, ExitPrice: 105.0,
		Pnl: 250.0, Reason: "MA crossover bullish", ExitReason: "target",
	}
	keys := jsonKeys(t, resp)
	want := []string{"entryPrice", "entryTs", "exitPrice", "exitReason", "exitTs", "pnl", "quantity", "reason", "side"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestListResponseJSONShape(t *testing.T) {
	resp := backtestListResponse{
		Runs:  []backtestSummaryResponse{},
		Page:  1,
		Limit: 20,
		Total: 0,
	}
	keys := jsonKeys(t, resp)
	want := []string{"limit", "page", "runs", "total"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestSummaryResponseJSONShape(t *testing.T) {
	resp := backtestSummaryResponse{
		ID:          "abc",
		Status:      models.BacktestCompleted,
		CompletedAt: "2025-01-02T09:30:00Z",
		Result: backtestSummaryResultResponse{
			Strategy: backtestStrategyResponse{},
		},
	}
	keys := jsonKeys(t, resp)
	want := []string{"completedAt", "id", "result", "status"}
	assertKeysEqual(t, keys, want)

	resultKeys := jsonKeys(t, resp.Result)
	resultWant := []string{
		"capEnd", "capStart", "from", "interval", "maxDrawdownPct", "netPnl",
		"returnPct", "strategy", "to", "tradeCount", "underlying", "winRate",
	}
	assertKeysEqual(t, resultKeys, resultWant)
}

func TestToBacktestStatusResponse_Completed(t *testing.T) {
	id := uuid.New()
	capital := 200000.0
	pnl := 22400.0
	total := 22
	wins := 11
	losses := 11
	dd := 0.05
	lots := 2
	slug := "ma_crossover"
	strategyName := "MA Crossover"
	underlying := "NIFTY"

	run := &models.BacktestRun{
		ID:           id,
		Status:       models.BacktestCompleted,
		NetPnl:       &pnl,
		TotalTrades:  &total,
		WinCount:     &wins,
		LossCount:    &losses,
		MaxDrawdown:  &dd,
		Capital:      &capital,
		Lots:         &lots,
		StrategySlug: &slug,
		StrategyName: &strategyName,
		Underlying:   &underlying,
		FromTs:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ToTs:         time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
	}

	resp := toBacktestStatusResponse(run)
	if resp.ID != id.String() {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}
	if resp.Status != models.BacktestCompleted {
		t.Errorf("expected status COMPLETED, got %s", resp.Status)
	}
	if resp.Result == nil {
		t.Fatal("expected Result to be non-nil for COMPLETED run")
	}
	r := resp.Result
	if math.Abs(r.ReturnPct-11.2) > floatTolerance {
		t.Errorf("expected returnPct 11.2, got %f", r.ReturnPct)
	}
	if r.WinRate != 50 {
		t.Errorf("expected winRate 50, got %d", r.WinRate)
	}
	if r.TradeCount != 22 {
		t.Errorf("expected tradeCount 22, got %d", r.TradeCount)
	}
	if r.Strategy.ID != slug {
		t.Errorf("expected strategy id %q, got %q", slug, r.Strategy.ID)
	}
	if r.Strategy.Name != strategyName {
		t.Errorf("expected strategy name %q, got %q", strategyName, r.Strategy.Name)
	}
	if math.Abs(r.CapEnd-(capital+pnl)) > floatTolerance {
		t.Errorf("expected capEnd %f, got %f", capital+pnl, r.CapEnd)
	}
	if math.Abs(r.MaxDrawdownPct-5.0) > floatTolerance {
		t.Errorf("expected maxDrawdownPct 5.0, got %f", r.MaxDrawdownPct)
	}
}

func TestToBacktestListResponse_CompletedSummary(t *testing.T) {
	id := uuid.New()
	capital := 200000.0
	pnl := 22400.0
	totalTrades := 22
	wins := 11
	maxDrawdown := 0.05
	slug := "ma_crossover"
	strategyName := "MA Crossover"
	underlying := "NIFTY"
	completedAt := time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC)

	resp := toBacktestListResponse([]models.BacktestRun{
		{
			ID:             id,
			Status:         models.BacktestCompleted,
			NetPnl:         &pnl,
			TotalTrades:    &totalTrades,
			WinCount:       &wins,
			MaxDrawdown:    &maxDrawdown,
			Capital:        &capital,
			StrategySlug:   &slug,
			StrategyName:   &strategyName,
			Underlying:     &underlying,
			CandleInterval: models.CandleInterval1D,
			FromTs:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			ToTs:           time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
			CompletedAt:    &completedAt,
		},
	}, 1, 20, 1)

	if resp.Page != 1 || resp.Limit != 20 || resp.Total != 1 {
		t.Fatalf("unexpected pagination: page=%d limit=%d total=%d", resp.Page, resp.Limit, resp.Total)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}
	run := resp.Runs[0]
	if run.ID != id.String() {
		t.Errorf("expected ID %s, got %s", id, run.ID)
	}
	if run.CompletedAt != completedAt.Format(time.RFC3339) {
		t.Errorf("expected completedAt %s, got %s", completedAt.Format(time.RFC3339), run.CompletedAt)
	}
	if run.Result.Strategy.ID != slug || run.Result.Strategy.Name != strategyName {
		t.Errorf("unexpected strategy: %+v", run.Result.Strategy)
	}
	if math.Abs(run.Result.CapEnd-(capital+pnl)) > floatTolerance {
		t.Errorf("expected capEnd %f, got %f", capital+pnl, run.Result.CapEnd)
	}
	if run.Result.WinRate == nil || *run.Result.WinRate != 50 {
		t.Fatalf("expected winRate 50, got %v", run.Result.WinRate)
	}
	if run.Result.MaxDrawdownPct == nil || math.Abs(*run.Result.MaxDrawdownPct-5.0) > floatTolerance {
		t.Fatalf("expected maxDrawdownPct 5.0, got %v", run.Result.MaxDrawdownPct)
	}
}

func TestToBacktestSummaryResponse_NullableOptionalMetrics(t *testing.T) {
	run := &models.BacktestRun{
		ID:          uuid.New(),
		Status:      models.BacktestCompleted,
		FromTs:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ToTs:        time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
		CompletedAt: ptrTime(time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC)),
	}

	resp := toBacktestSummaryResponse(run)
	if resp.Result.WinRate != nil {
		t.Fatalf("expected nil winRate, got %v", resp.Result.WinRate)
	}
	if resp.Result.MaxDrawdownPct != nil {
		t.Fatalf("expected nil maxDrawdownPct, got %v", resp.Result.MaxDrawdownPct)
	}
}

func TestToBacktestStatusResponse_Running(t *testing.T) {
	run := &models.BacktestRun{ID: uuid.New(), Status: models.BacktestRunning}
	resp := toBacktestStatusResponse(run)
	if resp.Result != nil {
		t.Error("Result should be nil for RUNNING run")
	}
	if resp.ErrorMessage != nil {
		t.Error("ErrorMessage should be nil for RUNNING run")
	}
}

func TestToBacktestStatusResponse_Failed(t *testing.T) {
	msg := "no candle data available"
	run := &models.BacktestRun{
		ID:           uuid.New(),
		Status:       models.BacktestFailed,
		ErrorMessage: &msg,
	}
	resp := toBacktestStatusResponse(run)
	if resp.Result != nil {
		t.Error("Result should be nil for FAILED run")
	}
	if resp.ErrorMessage == nil || *resp.ErrorMessage != msg {
		t.Errorf("expected errorMessage %q", msg)
	}
}

func TestToBacktestTradesPageResponse_Pagination(t *testing.T) {
	trades := make([]models.Trade, 5)
	for i := range trades {
		trades[i] = models.Trade{
			EntryTimestamp: time.Date(2025, 1, i+1, 9, 15, 0, 0, time.UTC),
			ExitTimestamp:  time.Date(2025, 1, i+1, 9, 30, 0, 0, time.UTC),
			Side:           models.OrderSideBuy,
			PnL:            float64(i+1) * 100,
		}
	}
	raw, _ := json.Marshal(trades)

	// page 1, limit 2 → first 2 trades
	resp, err := toBacktestTradesPageResponse(raw, 1, 2)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if resp.Total != 5 {
		t.Errorf("expected total 5, got %d", resp.Total)
	}
	if len(resp.Trades) != 2 {
		t.Errorf("expected 2 trades on page 1, got %d", len(resp.Trades))
	}

	// page 3, limit 2 → last 1 trade
	resp, err = toBacktestTradesPageResponse(raw, 3, 2)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if len(resp.Trades) != 1 {
		t.Errorf("expected 1 trade on page 3, got %d", len(resp.Trades))
	}

	// page beyond end → empty
	resp, err = toBacktestTradesPageResponse(raw, 10, 2)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if len(resp.Trades) != 0 {
		t.Errorf("expected 0 trades past end, got %d", len(resp.Trades))
	}
}

func TestBacktestSubmitRequestJSONShape(t *testing.T) {
	resp := backtestSubmitRequest{
		StrategyID: "ma_crossover",
		Inputs: backtestInputsRequest{
			Underlying: "NIFTY",
			DateRange:  backtestDateRange{From: "2025-01-01", To: "2025-06-30"},
			Lots:       2,
			Capital:    200000,
		},
	}
	keys := jsonKeys(t, resp)
	want := []string{"inputs", "strategyId"}
	assertKeysEqual(t, keys, want)
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
