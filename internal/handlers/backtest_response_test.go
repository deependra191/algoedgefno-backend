package handlers

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
				Equity: []chartPointResponse{},
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
			Equity: []chartPointResponse{},
		},
	}
	keys := jsonKeys(t, resp)
	want := []string{
		"avgHoldingMinutes", "avgLoss", "avgPnlPerTrade", "avgWin",
		"bestTrade", "capEnd", "capStart", "chart",
		"from", "grossPnl", "interval", "longestLossStreak", "longestWinStreak",
		"lots", "maxDrawdownPct", "netPnl", "profitFactor", "returnPct",
		"rewardRisk", "slippage", "strategy", "to",
		"totalCharges", "tradeCount", "tradesPerWeek", "underlying", "winRate", "worstTrade",
	}
	assertKeysEqual(t, keys, want)
}

func TestBacktestChartResponseJSONShape_OmitsDrawdown(t *testing.T) {
	resp := backtestChartResponse{
		Equity: []chartPointResponse{},
	}
	keys := jsonKeys(t, resp)
	want := []string{"equity"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestTradeResponseJSONShape(t *testing.T) {
	resp := backtestTradeResponse{
		EntryTs: "2025-01-02T09:15:00Z", ExitTs: "2025-01-02T09:30:00Z",
		Side: "BUY", Quantity: 50, EntryPrice: 100.0, ExitPrice: 105.0,
		Pnl: 250.0, Reason: "MA crossover bullish", ExitReason: "target",
	}
	keys := jsonKeys(t, resp)
	want := []string{
		"brokerage", "entryPrice", "entryTs", "exchangeFees", "exitPrice", "exitReason", "exitTs",
		"grossPnl", "gst", "pnl", "quantity", "reason", "sebiFees", "side", "slippage", "stampDuty",
		"stt", "totalCharges",
	}
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
		"capEnd", "capStart", "from", "grossPnl", "interval", "maxDrawdownPct", "netPnl",
		"returnPct", "slippage", "strategy", "to", "totalCharges", "tradeCount", "underlying", "winRate",
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
	run := validBacktestSummaryRun()
	resp, err := toBacktestListResponse([]models.BacktestRun{run}, 1, 20, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Page != 1 || resp.Limit != 20 || resp.Total != 1 {
		t.Fatalf("unexpected pagination: page=%d limit=%d total=%d", resp.Page, resp.Limit, resp.Total)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}
	got := resp.Runs[0]
	if got.ID != run.ID.String() {
		t.Errorf("expected ID %s, got %s", run.ID, got.ID)
	}
	if got.CompletedAt != run.CompletedAt.Format(time.RFC3339) {
		t.Errorf("expected completedAt %s, got %s", run.CompletedAt.Format(time.RFC3339), got.CompletedAt)
	}
	if got.Result.Strategy.ID != *run.StrategySlug || got.Result.Strategy.Name != *run.StrategyName {
		t.Errorf("unexpected strategy: %+v", got.Result.Strategy)
	}
	if math.Abs(got.Result.CapEnd-(*run.Capital+*run.NetPnl)) > floatTolerance {
		t.Errorf("expected capEnd %f, got %f", *run.Capital+*run.NetPnl, got.Result.CapEnd)
	}
	if got.Result.WinRate == nil || *got.Result.WinRate != 50 {
		t.Fatalf("expected winRate 50, got %v", got.Result.WinRate)
	}
	if got.Result.MaxDrawdownPct == nil || math.Abs(*got.Result.MaxDrawdownPct-5.0) > floatTolerance {
		t.Fatalf("expected maxDrawdownPct 5.0, got %v", got.Result.MaxDrawdownPct)
	}
}

func TestToBacktestSummaryResponse_RequiredFieldValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.BacktestRun)
	}{
		{name: "missing strategy slug", mutate: func(r *models.BacktestRun) { r.StrategySlug = nil }},
		{name: "blank strategy slug", mutate: func(r *models.BacktestRun) { r.StrategySlug = ptrString("") }},
		{name: "missing strategy name", mutate: func(r *models.BacktestRun) { r.StrategyName = nil }},
		{name: "blank strategy name", mutate: func(r *models.BacktestRun) { r.StrategyName = ptrString("") }},
		{name: "missing underlying", mutate: func(r *models.BacktestRun) { r.Underlying = nil }},
		{name: "blank underlying", mutate: func(r *models.BacktestRun) { r.Underlying = ptrString("") }},
		{name: "missing capital", mutate: func(r *models.BacktestRun) { r.Capital = nil }},
		{name: "missing net pnl", mutate: func(r *models.BacktestRun) { r.NetPnl = nil }},
		{name: "missing total trades", mutate: func(r *models.BacktestRun) { r.TotalTrades = nil }},
		{name: "missing completed at", mutate: func(r *models.BacktestRun) { r.CompletedAt = nil }},
		{name: "nil id", mutate: func(r *models.BacktestRun) { r.ID = uuid.Nil }},
		{name: "non completed status", mutate: func(r *models.BacktestRun) { r.Status = models.BacktestRunning }},
		{name: "missing interval", mutate: func(r *models.BacktestRun) { r.CandleInterval = "" }},
		{name: "missing from", mutate: func(r *models.BacktestRun) { r.FromTs = time.Time{} }},
		{name: "missing to", mutate: func(r *models.BacktestRun) { r.ToTs = time.Time{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := validBacktestSummaryRun()
			tt.mutate(&run)
			if _, err := toBacktestSummaryResponse(&run); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestToBacktestSummaryResponse_NullableOptionalMetrics(t *testing.T) {
	run := validBacktestSummaryRun()
	run.WinCount = nil
	run.MaxDrawdown = nil

	resp, err := toBacktestSummaryResponse(&run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Result.WinRate != nil {
		t.Fatalf("expected nil winRate, got %v", resp.Result.WinRate)
	}
	if resp.Result.MaxDrawdownPct != nil {
		t.Fatalf("expected nil maxDrawdownPct, got %v", resp.Result.MaxDrawdownPct)
	}
}

func TestToBacktestSummaryResponse_WinRateNilWhenNoTrades(t *testing.T) {
	run := validBacktestSummaryRun()
	*run.TotalTrades = 0

	resp, err := toBacktestSummaryResponse(&run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Result.WinRate != nil {
		t.Fatalf("expected nil winRate, got %v", resp.Result.WinRate)
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
			GrossPnL:       float64(i+1) * 100,
			TotalCharges:   float64(i+1) * 5,
			NetPnL:         float64(i+1)*100 - float64(i+1)*5,
		}
	}

	// page 1, limit 2 → first 2 trades
	resp, err := toBacktestTradesPageResponse(trades, 1, 2)
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
	resp, err = toBacktestTradesPageResponse(trades, 3, 2)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if len(resp.Trades) != 1 {
		t.Errorf("expected 1 trade on page 3, got %d", len(resp.Trades))
	}

	// page beyond end → empty
	resp, err = toBacktestTradesPageResponse(trades, 10, 2)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if len(resp.Trades) != 0 {
		t.Errorf("expected 0 trades past end, got %d", len(resp.Trades))
	}
}

// TestToBacktestTradesPageResponse_ChargeInvariant guarantees each emitted trade
// satisfies pnl == grossPnl − totalCharges − slippage within float-rounding tolerance.
// Catches reordering of mapper fields and accidental Pnl ← GrossPnL wiring.
func TestToBacktestTradesPageResponse_ChargeInvariant(t *testing.T) {
	tr := models.Trade{
		EntryTimestamp: time.Date(2025, 1, 2, 9, 15, 0, 0, time.UTC),
		ExitTimestamp:  time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC),
		Side:           models.OrderSideBuy,
		Quantity:       50,
		EntryPrice:     100,
		ExitPrice:      120,
		GrossPnL:       1000,
		Slippage:       11,
		Brokerage:      40,
		STT:            9,
		ExchangeFees:   3.91,
		SEBIFees:       0.011,
		GST:            7.91,
		StampDuty:      0.15,
		TotalCharges:   61,
		NetPnL:         928,
	}

	resp, err := toBacktestTradesPageResponse([]models.Trade{tr}, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(resp.Trades))
	}
	got := resp.Trades[0]
	if math.Abs(got.Pnl-(got.GrossPnl-got.TotalCharges-got.Slippage)) > 0.01 {
		t.Errorf("invariant violated: pnl=%.4f grossPnl=%.4f totalCharges=%.4f slippage=%.4f",
			got.Pnl, got.GrossPnl, got.TotalCharges, got.Slippage)
	}
	if got.Pnl != tr.NetPnL {
		t.Errorf("pnl JSON field must be sourced from NetPnL: got=%.4f want=%.4f", got.Pnl, tr.NetPnL)
	}
}

// TestToBacktestSummaryResponse_PreB14NilCharges asserts that runs persisted before
// the B14 migration (GrossPnl/TotalCharges columns NULL) still render successfully,
// with both fields rendering as 0 in the response.
func TestToBacktestSummaryResponse_PreB14NilCharges(t *testing.T) {
	run := validBacktestSummaryRun()
	run.GrossPnl = nil
	run.TotalCharges = nil
	run.Slippage = nil

	resp, err := toBacktestSummaryResponse(&run)
	if err != nil {
		t.Fatalf("expected nil-charge run to render successfully, got error: %v", err)
	}
	if resp.Result.GrossPnl != 0 {
		t.Errorf("expected grossPnl 0 for pre-B14 run, got %v", resp.Result.GrossPnl)
	}
	if resp.Result.TotalCharges != 0 {
		t.Errorf("expected totalCharges 0 for pre-B14 run, got %v", resp.Result.TotalCharges)
	}
	if resp.Result.Slippage != 0 {
		t.Errorf("expected slippage 0 for pre-B14 run, got %v", resp.Result.Slippage)
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

func TestParseBacktestListPagination(t *testing.T) {
	tests := []struct {
		name      string
		rawQuery  string
		wantPage  int
		wantLimit int
	}{
		{name: "defaults", wantPage: 1, wantLimit: 20},
		{name: "custom", rawQuery: "page=2&limit=30", wantPage: 2, wantLimit: 30},
		{name: "caps limit", rawQuery: "page=3&limit=500", wantPage: 3, wantLimit: 100},
		{name: "invalid falls back", rawQuery: "page=abc&limit=xyz", wantPage: 1, wantLimit: 20},
		{name: "negative falls back", rawQuery: "page=-1&limit=-20", wantPage: 1, wantLimit: 20},
		{name: "zero falls back", rawQuery: "page=0&limit=0", wantPage: 1, wantLimit: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, limit := parseBacktestListPagination(testGinContext(tt.rawQuery))
			if page != tt.wantPage || limit != tt.wantLimit {
				t.Fatalf("expected page=%d limit=%d, got page=%d limit=%d", tt.wantPage, tt.wantLimit, page, limit)
			}
		})
	}
}

func TestParseTradesPagination(t *testing.T) {
	tests := []struct {
		name      string
		rawQuery  string
		wantPage  int
		wantLimit int
	}{
		{name: "defaults", wantPage: 1, wantLimit: 50},
		{name: "custom", rawQuery: "page=2&limit=75", wantPage: 2, wantLimit: 75},
		{name: "caps limit", rawQuery: "page=3&limit=500", wantPage: 3, wantLimit: 200},
		{name: "invalid falls back", rawQuery: "page=abc&limit=xyz", wantPage: 1, wantLimit: 50},
		{name: "negative falls back", rawQuery: "page=-1&limit=-20", wantPage: 1, wantLimit: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, limit := parseTradesPagination(testGinContext(tt.rawQuery))
			if page != tt.wantPage || limit != tt.wantLimit {
				t.Fatalf("expected page=%d limit=%d, got page=%d limit=%d", tt.wantPage, tt.wantLimit, page, limit)
			}
		})
	}
}

func validBacktestSummaryRun() models.BacktestRun {
	return models.BacktestRun{
		ID:             uuid.New(),
		Status:         models.BacktestCompleted,
		NetPnl:         ptrFloat(22400),
		TotalTrades:    ptrInt(22),
		WinCount:       ptrInt(11),
		MaxDrawdown:    ptrFloat(0.05),
		Capital:        ptrFloat(200000),
		StrategySlug:   ptrString("ma_crossover"),
		StrategyName:   ptrString("MA Crossover"),
		Underlying:     ptrString("NIFTY"),
		CandleInterval: models.CandleInterval1D,
		FromTs:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ToTs:           time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
		CompletedAt:    ptrTime(time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC)),
	}
}

func testGinContext(rawQuery string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	target := "/api/v1/backtests"
	if strings.TrimSpace(rawQuery) != "" {
		target += "?" + rawQuery
	}
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}

func ptrString(v string) *string {
	return &v
}

func ptrFloat(v float64) *float64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
