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

func TestBuildChartData_Empty(t *testing.T) {
	cd := BuildChartData(nil, 0)
	if len(cd.Equity) != 0 || len(cd.Drawdown) != 0 {
		t.Error("expected empty slices for nil trades")
	}
}

func TestBuildChartData_EquityCurve(t *testing.T) {
	trades := []models.Trade{
		makeTrade(0, 30, 1000),
		makeTrade(30, 60, -200),
		makeTrade(60, 90, 500),
	}
	cd := BuildChartData(trades, 0)

	if len(cd.Equity) != 3 {
		t.Fatalf("expected 3 equity points, got %d", len(cd.Equity))
	}
	if cd.Equity[0].Value != 1000 {
		t.Errorf("equity[0] want 1000, got %f", cd.Equity[0].Value)
	}
	if cd.Equity[1].Value != 800 {
		t.Errorf("equity[1] want 800, got %f", cd.Equity[1].Value)
	}
	if cd.Equity[2].Value != 1300 {
		t.Errorf("equity[2] want 1300, got %f", cd.Equity[2].Value)
	}
}

func TestBuildChartData_DrawdownAtPeak(t *testing.T) {
	// Each trade is profitable — always at peak, drawdown always 0.
	trades := []models.Trade{
		makeTrade(0, 30, 500),
		makeTrade(30, 60, 300),
	}
	cd := BuildChartData(trades, 0)

	for i, pt := range cd.Drawdown {
		if pt.Value != 0 {
			t.Errorf("drawdown[%d] want 0 at peak, got %f", i, pt.Value)
		}
	}
}

func TestBuildChartData_DrawdownCalculation(t *testing.T) {
	// Peak at 1000 after first trade, then drops to 800 → drawdown = 20%
	trades := []models.Trade{
		makeTrade(0, 30, 1000),
		makeTrade(30, 60, -200),
	}
	cd := BuildChartData(trades, 0)

	if cd.Drawdown[0].Value != 0 {
		t.Errorf("drawdown[0] want 0 (at peak), got %f", cd.Drawdown[0].Value)
	}
	expectedDD := round2(200.0 / 1000.0 * 100) // 20%
	if cd.Drawdown[1].Value != expectedDD {
		t.Errorf("drawdown[1] want %f, got %f", expectedDD, cd.Drawdown[1].Value)
	}
}

func TestBuildChartData_DrawdownFromZero(t *testing.T) {
	// Equity starts negative (all losses) — drawdown must be non-zero, expressed
	// relative to capital, not the zero peak.
	trades := []models.Trade{
		makeTrade(0, 30, -500),
		makeTrade(30, 60, -300),
	}
	capital := 10000.0
	cd := BuildChartData(trades, capital)

	// After first trade: equity=-500, peak=0, dd = 500/10000*100 = 5%
	expectedDD0 := round2(500.0 / capital * 100)
	if cd.Drawdown[0].Value != expectedDD0 {
		t.Errorf("drawdown[0] want %f, got %f", expectedDD0, cd.Drawdown[0].Value)
	}
	// After second trade: equity=-800, peak=0, dd = 800/10000*100 = 8%
	expectedDD1 := round2(800.0 / capital * 100)
	if cd.Drawdown[1].Value != expectedDD1 {
		t.Errorf("drawdown[1] want %f, got %f", expectedDD1, cd.Drawdown[1].Value)
	}
}

func TestBuildChartData_TimestampsAreExitTs(t *testing.T) {
	exit1 := t0.Add(30 * time.Minute)
	exit2 := t0.Add(60 * time.Minute)
	trades := []models.Trade{
		{EntryTimestamp: t0, ExitTimestamp: exit1, NetPnL: 100},
		{EntryTimestamp: exit1, ExitTimestamp: exit2, NetPnL: 200},
	}
	cd := BuildChartData(trades, 0)

	if !cd.Equity[0].Ts.Equal(exit1) {
		t.Errorf("equity[0].Ts want %v, got %v", exit1, cd.Equity[0].Ts)
	}
	if !cd.Equity[1].Ts.Equal(exit2) {
		t.Errorf("equity[1].Ts want %v, got %v", exit2, cd.Equity[1].Ts)
	}
}
