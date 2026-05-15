package engine

import (
	"testing"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var (
	t0  = time.Date(2025, 1, 2, 9, 15, 0, 0, time.UTC)
	tTo = time.Date(2025, 12, 31, 15, 30, 0, 0, time.UTC)
)

func makeTrade(entryMin, exitMin int, pnl float64) models.Trade {
	return models.Trade{
		EntryTimestamp: t0.Add(time.Duration(entryMin) * time.Minute),
		ExitTimestamp:  t0.Add(time.Duration(exitMin) * time.Minute),
		NetPnL:         pnl,
	}
}

func TestComputeTradeStats_Empty(t *testing.T) {
	s := ComputeTradeStats(nil, t0, tTo)
	if s.AvgWin != nil || s.AvgLoss != nil || s.ProfitFactor != nil || s.RewardRisk != nil {
		t.Error("all ratio fields should be nil for empty trade list")
	}
	if s.LongestWinStreak != 0 || s.LongestLossStreak != 0 {
		t.Error("streaks should be 0 for empty trade list")
	}
	if s.TradesPerWeek != 0 {
		t.Error("tradesPerWeek should be 0 for empty trade list")
	}
}

func TestComputeTradeStats_AllWins(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 30, 500),
		makeTrade(60, 90, 300),
		makeTrade(120, 180, 700),
	}
	s := ComputeTradeStats(trades, t0, tTo)

	if s.AvgWin == nil || *s.AvgWin != round2((500+300+700)/3.0) {
		t.Errorf("unexpected AvgWin: %v", s.AvgWin)
	}
	if s.AvgLoss != nil {
		t.Error("AvgLoss should be nil when no losses")
	}
	if s.ProfitFactor != nil {
		t.Error("ProfitFactor should be nil when no losses")
	}
	if s.RewardRisk != nil {
		t.Error("RewardRisk should be nil when no losses")
	}
	if s.LongestWinStreak != 3 {
		t.Errorf("expected longestWinStreak 3, got %d", s.LongestWinStreak)
	}
	if s.LongestLossStreak != 0 {
		t.Errorf("expected longestLossStreak 0, got %d", s.LongestLossStreak)
	}
	if *s.BestTrade != 700 {
		t.Errorf("expected best 700, got %v", *s.BestTrade)
	}
	if *s.WorstTrade != 300 {
		t.Errorf("expected worst 300, got %v", *s.WorstTrade)
	}
}

func TestComputeTradeStats_AllLosses(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 30, -200),
		makeTrade(60, 90, -400),
	}
	s := ComputeTradeStats(trades, t0, tTo)

	if s.AvgWin != nil {
		t.Error("AvgWin should be nil when no wins")
	}
	if s.AvgLoss == nil || *s.AvgLoss != round2((-200-400)/2.0) {
		t.Errorf("unexpected AvgLoss: %v", s.AvgLoss)
	}
	if s.ProfitFactor != nil {
		t.Error("ProfitFactor should be nil when no wins")
	}
	if s.LongestLossStreak != 2 {
		t.Errorf("expected longestLossStreak 2, got %d", s.LongestLossStreak)
	}
}

func TestComputeTradeStats_Mixed(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 60, 1000),
		makeTrade(60, 90, -200),
		makeTrade(90, 150, 800),
		makeTrade(150, 180, -100),
		makeTrade(180, 240, 600),
	}
	s := ComputeTradeStats(trades, t0, tTo)

	if s.AvgWin == nil {
		t.Fatal("AvgWin should not be nil")
	}
	if s.AvgLoss == nil {
		t.Fatal("AvgLoss should not be nil")
	}
	if s.ProfitFactor == nil {
		t.Fatal("ProfitFactor should not be nil")
	}
	if s.RewardRisk == nil {
		t.Fatal("RewardRisk should not be nil")
	}
	expectedPF := round2(2400.0 / 300.0)
	if *s.ProfitFactor != expectedPF {
		t.Errorf("expected ProfitFactor %f, got %f", expectedPF, *s.ProfitFactor)
	}
	// Pattern W,L,W,L,W — no consecutive wins, so longest win streak = 1.
	if s.LongestWinStreak != 1 {
		t.Errorf("expected longestWinStreak 1, got %d", s.LongestWinStreak)
	}
	if s.LongestLossStreak != 1 {
		t.Errorf("expected longestLossStreak 1, got %d", s.LongestLossStreak)
	}
}

func TestComputeTradeStats_AvgHolding(t *testing.T) {
	// Two trades: 30 min and 90 min holding → avg 60 min
	trades := []models.Trade{
		makeTrade(0, 30, 100),
		makeTrade(60, 150, -50),
	}
	s := ComputeTradeStats(trades, t0, tTo)
	if s.AvgHoldingMinutes == nil || *s.AvgHoldingMinutes != 60 {
		t.Errorf("expected avg holding 60 min, got %v", s.AvgHoldingMinutes)
	}
}

func TestComputeTradeStats_Streaks(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 10, 100),
		makeTrade(10, 20, 200),
		makeTrade(20, 30, 300),
		makeTrade(30, 40, -50),
		makeTrade(40, 50, -60),
		makeTrade(50, 60, 100),
	}
	s := ComputeTradeStats(trades, t0, tTo)
	if s.LongestWinStreak != 3 {
		t.Errorf("expected longestWinStreak 3, got %d", s.LongestWinStreak)
	}
	if s.LongestLossStreak != 2 {
		t.Errorf("expected longestLossStreak 2, got %d", s.LongestLossStreak)
	}
}

// Chart tests use a 120-minute run window; capital defaults to 100000.
// All cases verify both equity (running balance = capital + cumPnL) and
// drawdown (% from running peak, peak seeded at capital).

const chartCapital = 100000.0

var chartTo = t0.Add(120 * time.Minute)

func assertPoint(t *testing.T, label string, got models.ChartPoint, wantTs time.Time, wantVal float64) {
	t.Helper()
	if !got.Ts.Equal(wantTs) {
		t.Errorf("%s.Ts: want %v, got %v", label, wantTs, got.Ts)
	}
	if got.Value != wantVal {
		t.Errorf("%s.Value: want %f, got %f", label, wantVal, got.Value)
	}
}

func TestBuildChartData_ZeroTrades_EmitsTwoFlatEndpoints(t *testing.T) {
	cd := BuildChartData(nil, chartCapital, t0, chartTo)

	if len(cd.Equity) != 2 || len(cd.Drawdown) != 2 {
		t.Fatalf("want 2 points each, got equity=%d drawdown=%d", len(cd.Equity), len(cd.Drawdown))
	}
	assertPoint(t, "equity[0]", cd.Equity[0], t0, chartCapital)
	assertPoint(t, "equity[1]", cd.Equity[1], chartTo, chartCapital)
	assertPoint(t, "drawdown[0]", cd.Drawdown[0], t0, 0)
	assertPoint(t, "drawdown[1]", cd.Drawdown[1], chartTo, 0)
}

func TestBuildChartData_SingleTrade_ExitBeforeTo_EmitsThreePoints(t *testing.T) {
	trades := []models.Trade{makeTrade(0, 30, 500)}
	exit := t0.Add(30 * time.Minute)
	cd := BuildChartData(trades, chartCapital, t0, chartTo)

	if len(cd.Equity) != 3 {
		t.Fatalf("want 3 equity points, got %d", len(cd.Equity))
	}
	assertPoint(t, "equity[0]", cd.Equity[0], t0, chartCapital)
	assertPoint(t, "equity[1]", cd.Equity[1], exit, chartCapital+500)
	assertPoint(t, "equity[2]", cd.Equity[2], chartTo, chartCapital+500)
	// All points at peak (rising equity) → drawdown 0 throughout.
	for i, pt := range cd.Drawdown {
		if pt.Value != 0 {
			t.Errorf("drawdown[%d] want 0, got %f", i, pt.Value)
		}
	}
}

func TestBuildChartData_SingleTrade_ExitEqualsTo_SuppressesTerminal(t *testing.T) {
	trades := []models.Trade{makeTrade(0, 120, -500)}
	cd := BuildChartData(trades, chartCapital, t0, chartTo)

	if len(cd.Equity) != 2 {
		t.Fatalf("want 2 equity points (terminal suppressed), got %d", len(cd.Equity))
	}
	assertPoint(t, "equity[0]", cd.Equity[0], t0, chartCapital)
	assertPoint(t, "equity[1]", cd.Equity[1], chartTo, chartCapital-500)
	// Drawdown: peak=chartCapital, eq=chartCapital-500, dd = 500/100000 = 0.5%.
	expectedDD := round2(500.0 / chartCapital * 100)
	assertPoint(t, "drawdown[0]", cd.Drawdown[0], t0, 0)
	assertPoint(t, "drawdown[1]", cd.Drawdown[1], chartTo, expectedDD)
}

func TestBuildChartData_MultipleTrades_LastExitBeforeTo_EmitsTerminal(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 30, 1000),
		makeTrade(30, 60, -200),
		makeTrade(60, 90, 500),
	}
	cd := BuildChartData(trades, chartCapital, t0, chartTo)

	if len(cd.Equity) != 5 {
		t.Fatalf("want 5 equity points (seed + 3 trades + terminal), got %d", len(cd.Equity))
	}
	assertPoint(t, "equity[0]", cd.Equity[0], t0, chartCapital)
	assertPoint(t, "equity[1]", cd.Equity[1], t0.Add(30*time.Minute), chartCapital+1000)
	assertPoint(t, "equity[2]", cd.Equity[2], t0.Add(60*time.Minute), chartCapital+800)
	assertPoint(t, "equity[3]", cd.Equity[3], t0.Add(90*time.Minute), chartCapital+1300)
	assertPoint(t, "equity[4]", cd.Equity[4], chartTo, chartCapital+1300)
	// Drawdown: peak hits chartCapital+1000 at t+30, then equity dips to +800
	// → dd = 200/101000 = 0.198... ≈ 0.20. Then equity reclaims peak.
	expectedDip := round2(200.0 / (chartCapital + 1000) * 100)
	assertPoint(t, "drawdown[2]", cd.Drawdown[2], t0.Add(60*time.Minute), expectedDip)
	if cd.Drawdown[3].Value != 0 || cd.Drawdown[4].Value != 0 {
		t.Errorf("drawdown should return to 0 once peak reclaimed; got %f %f",
			cd.Drawdown[3].Value, cd.Drawdown[4].Value)
	}
}

func TestBuildChartData_MultipleTrades_LastExitEqualsTo_SuppressesTerminal(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 30, 1000),
		makeTrade(30, 120, -300),
	}
	cd := BuildChartData(trades, chartCapital, t0, chartTo)

	if len(cd.Equity) != 3 {
		t.Fatalf("want 3 equity points (terminal suppressed), got %d", len(cd.Equity))
	}
	assertPoint(t, "equity[0]", cd.Equity[0], t0, chartCapital)
	assertPoint(t, "equity[1]", cd.Equity[1], t0.Add(30*time.Minute), chartCapital+1000)
	assertPoint(t, "equity[2]", cd.Equity[2], chartTo, chartCapital+700)
}

func TestBuildChartData_DrawdownArithmetic_KnownSequence(t *testing.T) {
	// Hand-verified: capital=100000, trades=[+5000, -3000].
	// Equity: [100000, 105000, 102000, 102000].
	// Peak: 100000 → 105000 → 105000.
	// Drawdown: [0, 0, 3000/105000*100=2.857..., same]. round2 → 2.86.
	trades := []models.Trade{
		makeTrade(0, 30, 5000),
		makeTrade(30, 60, -3000),
	}
	cd := BuildChartData(trades, chartCapital, t0, chartTo)

	if len(cd.Equity) != 4 {
		t.Fatalf("want 4 equity points, got %d", len(cd.Equity))
	}
	wantEq := []float64{100000, 105000, 102000, 102000}
	for i, want := range wantEq {
		if cd.Equity[i].Value != want {
			t.Errorf("equity[%d]: want %f, got %f", i, want, cd.Equity[i].Value)
		}
	}
	expectedDD := round2(3000.0 / 105000.0 * 100)
	wantDD := []float64{0, 0, expectedDD, expectedDD}
	for i, want := range wantDD {
		if cd.Drawdown[i].Value != want {
			t.Errorf("drawdown[%d]: want %f, got %f", i, want, cd.Drawdown[i].Value)
		}
	}
}

func TestBuildChartData_InvalidWindow_ReturnsEmpty(t *testing.T) {
	cd := BuildChartData(nil, chartCapital, chartTo, t0) // from > to
	if len(cd.Equity) != 0 || len(cd.Drawdown) != 0 {
		t.Errorf("want empty slices for invalid window, got equity=%d drawdown=%d",
			len(cd.Equity), len(cd.Drawdown))
	}
}

func TestBuildChartData_NonPositiveCapital_EmitsZeroDrawdowns(t *testing.T) {
	// Defensive: production callers always pass capital > 0, but tests pass 0.
	// Result must not panic; drawdown values are zero throughout.
	trades := []models.Trade{makeTrade(0, 30, -100)}
	cd := BuildChartData(trades, 0, t0, chartTo)

	for i, pt := range cd.Drawdown {
		if pt.Value != 0 {
			t.Errorf("drawdown[%d]: want 0 with capital=0, got %f", i, pt.Value)
		}
	}
}
