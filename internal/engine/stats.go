package engine

import (
	"math"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// ComputeTradeStats derives all presentation-layer statistics from a completed trade list.
// from and to are the backtest date range, used to compute tradesPerWeek.
// Pointer fields in the result are nil when mathematically undefined
// (e.g. no losing trades → ProfitFactor is nil).
func ComputeTradeStats(trades []models.Trade, from, to time.Time) *models.TradeStats {
	stats := &models.TradeStats{}
	if len(trades) == 0 {
		return stats
	}

	var grossWins, grossLosses float64
	var totalHolding float64
	var winSum, lossSum float64
	winCount, lossCount := 0, 0
	best := trades[0].NetPnL
	worst := trades[0].NetPnL

	for _, tr := range trades {
		if tr.NetPnL > best {
			best = tr.NetPnL
		}
		if tr.NetPnL < worst {
			worst = tr.NetPnL
		}
		totalHolding += tr.ExitTimestamp.Sub(tr.EntryTimestamp).Minutes()
		if tr.NetPnL > 0 {
			winSum += tr.NetPnL
			grossWins += tr.NetPnL
			winCount++
		} else if tr.NetPnL < 0 {
			lossSum += tr.NetPnL
			grossLosses += tr.NetPnL
			lossCount++
		}
	}

	n := len(trades)
	totalPnL := grossWins + grossLosses

	stats.BestTrade = pf(round2(best))
	stats.WorstTrade = pf(round2(worst))
	stats.AvgPnlPerTrade = pf(round2(totalPnL / float64(n)))
	stats.AvgHoldingMinutes = pf(round2(totalHolding / float64(n)))

	if winCount > 0 {
		stats.AvgWin = pf(round2(winSum / float64(winCount)))
	}
	if lossCount > 0 {
		stats.AvgLoss = pf(round2(lossSum / float64(lossCount)))
	}
	// ProfitFactor requires both wins and losses; undefined otherwise.
	if grossWins > 0 && grossLosses < 0 {
		stats.ProfitFactor = pf(round2(grossWins / math.Abs(grossLosses)))
	}
	// RewardRisk requires both a winning and a losing average.
	if stats.AvgWin != nil && stats.AvgLoss != nil && *stats.AvgLoss != 0 {
		stats.RewardRisk = pf(round2(*stats.AvgWin / math.Abs(*stats.AvgLoss)))
	}

	stats.LongestWinStreak, stats.LongestLossStreak = computeStreaks(trades)
	stats.TradesPerWeek = computeTradesPerWeek(n, from, to)

	return stats
}

// BuildChartData implements models.BacktestEngine. The legacy capital-fallback
// branch from the cumPnL-from-zero contract is gone: with peak seeded at capital,
// peak >= capital > 0 is invariant for the normal call path, so the branch was
// dead. The capital <= 0 case (only reachable from tests) emits zero drawdowns.
func BuildChartData(trades []models.Trade, capital float64, from, to time.Time) *models.ChartData {
	if !from.Before(to) {
		return &models.ChartData{Equity: []models.ChartPoint{}, Drawdown: []models.ChartPoint{}}
	}

	equity := make([]models.ChartPoint, 0, len(trades)+2)
	drawdown := make([]models.ChartPoint, 0, len(trades)+2)

	peak := capital
	equity = append(equity, models.ChartPoint{Ts: from, Value: round2(capital)})
	drawdown = append(drawdown, models.ChartPoint{Ts: from, Value: 0})

	var cumPnL float64
	var lastExit time.Time
	for _, tr := range trades {
		cumPnL += tr.NetPnL
		eq := capital + cumPnL
		if eq > peak {
			peak = eq
		}
		dd := 0.0
		if peak > 0 {
			dd = round2((peak - eq) / peak * 100)
		}
		equity = append(equity, models.ChartPoint{Ts: tr.ExitTimestamp, Value: round2(eq)})
		drawdown = append(drawdown, models.ChartPoint{Ts: tr.ExitTimestamp, Value: dd})
		lastExit = tr.ExitTimestamp
	}

	if len(trades) == 0 || !lastExit.Equal(to) {
		finalEq := capital + cumPnL
		dd := 0.0
		if peak > 0 {
			dd = round2((peak - finalEq) / peak * 100)
		}
		equity = append(equity, models.ChartPoint{Ts: to, Value: round2(finalEq)})
		drawdown = append(drawdown, models.ChartPoint{Ts: to, Value: dd})
	}

	return &models.ChartData{Equity: equity, Drawdown: drawdown}
}

// ComputeTradeStats implements models.BacktestEngine.
func (b *Backtester) ComputeTradeStats(trades []models.Trade, from, to time.Time) *models.TradeStats {
	return ComputeTradeStats(trades, from, to)
}

// BuildChartData implements models.BacktestEngine.
func (b *Backtester) BuildChartData(trades []models.Trade, capital float64, from, to time.Time) *models.ChartData {
	return BuildChartData(trades, capital, from, to)
}

func computeStreaks(trades []models.Trade) (longestWin, longestLoss int) {
	curWin, curLoss := 0, 0
	for _, tr := range trades {
		switch {
		case tr.NetPnL > 0:
			curWin++
			curLoss = 0
			if curWin > longestWin {
				longestWin = curWin
			}
		case tr.NetPnL < 0:
			curLoss++
			curWin = 0
			if curLoss > longestLoss {
				longestLoss = curLoss
			}
		default:
			curWin = 0
			curLoss = 0
		}
	}
	return
}

// computeTradesPerWeek divides trade count by the calendar-week span of the
// backtest range. Returns 0 if the range is zero or negative.
func computeTradesPerWeek(n int, from, to time.Time) float64 {
	weeks := to.Sub(from).Hours() / (24 * 7)
	if weeks <= 0 {
		return 0
	}
	return round2(float64(n) / weeks)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// pf returns a pointer to a float64 value.
func pf(v float64) *float64 { return &v }
