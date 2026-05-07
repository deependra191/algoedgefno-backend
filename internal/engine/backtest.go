package engine

import (
	"fmt"
	"math"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	ExitReasonTarget         = "target"
	ExitReasonStopLoss       = "stop_loss"
	ExitReasonTimeExit       = "time_exit"
	ExitReasonSignalReversal = "signal_reversal"
	ExitReasonEndOfData      = "end_of_data"
)

var _ models.BacktestEngine = (*Backtester)(nil)

// Backtester is the standard implementation of models.BacktestEngine.
type Backtester struct{}

func NewBacktester() *Backtester {
	return &Backtester{}
}

// RunBacktest simulates a strategy against historical signal and trade candles.
// Signals are generated from completed signal bars and executed at the open of
// the next eligible trade bar. Exit checks and end-of-data closure are driven by
// the trade-side candle stream.
// capital is the user's starting capital used as the drawdown denominator when
// the equity curve has not yet risen above zero (avoids division by zero).
func (b *Backtester) RunBacktest(strategy *models.Strategy, inputs models.EngineInputs, capital float64) (*models.BacktestResult, error) {
	if len(inputs.SignalCandles) == 0 || len(inputs.TradeCandles) == 0 {
		return &models.BacktestResult{}, nil
	}

	signals, err := Evaluate(strategy, inputs.SignalCandles)
	if err != nil {
		return nil, err
	}
	interval, err := intervalDuration(inputs.Interval)
	if err != nil {
		return nil, err
	}
	scheduledSignals := scheduleSignals(signals, inputs.TradeCandles, interval)

	lotSize := strategy.LotSize
	if lotSize <= 0 {
		lotSize = 1
	}
	nLots := strategy.NumberOfLots
	if nLots <= 0 {
		nLots = 1
	}
	qty := lotSize * nLots

	result := &models.BacktestResult{}
	var openTrade *models.Trade
	sigIdx := 0
	equity, peakEquity, maxDrawdown := 0.0, 0.0, 0.0

	for i := range inputs.TradeCandles {
		candle := &inputs.TradeCandles[i]

		if openTrade != nil {
			if reason, price := checkExitConditions(openTrade, candle, strategy); reason != "" {
				closeTrade(openTrade, price, candle.Timestamp, reason, qty)
				recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown, capital)
				openTrade = nil
			}
		}

		for sigIdx < len(scheduledSignals) && scheduledSignals[sigIdx].ExecutionTimestamp.Equal(candle.Timestamp) {
			sig := scheduledSignals[sigIdx]
			sigIdx++

			if openTrade != nil {
				if !isReversal(openTrade.Side, sig.Signal.Side) {
					continue
				}
				closeTrade(openTrade, candle.Open, candle.Timestamp, ExitReasonSignalReversal, qty)
				recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown, capital)
				openTrade = nil
			}

			openTrade = &models.Trade{
				EntryTimestamp: candle.Timestamp,
				Side:           sig.Signal.Side,
				Quantity:       qty,
				EntryPrice:     candle.Open,
				Reason:         sig.Signal.Reason,
			}
		}
	}

	if openTrade != nil && len(inputs.TradeCandles) > 0 {
		last := inputs.TradeCandles[len(inputs.TradeCandles)-1]
		closeTrade(openTrade, last.Close, last.Timestamp, ExitReasonEndOfData, qty)
		recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown, capital)
	}

	result.TotalTrades = len(result.Trades)
	for _, tr := range result.Trades {
		result.NetPnL += tr.PnL
		if tr.PnL > 0 {
			result.WinCount++
		} else if tr.PnL < 0 {
			result.LossCount++
		}
	}
	result.MaxDrawdown = math.Round(maxDrawdown*10000) / 10000

	return result, nil
}

type scheduledSignal struct {
	Signal             Signal
	ExecutionTimestamp time.Time
}

func scheduleSignals(signals []Signal, tradeCandles []models.Candle, interval time.Duration) []scheduledSignal {
	if len(signals) == 0 || len(tradeCandles) == 0 {
		return nil
	}

	scheduled := make([]scheduledSignal, 0, len(signals))
	matchTradeIdx := 0
	executionTradeIdx := 0
	for _, sig := range signals {
		for matchTradeIdx < len(tradeCandles) && tradeCandles[matchTradeIdx].Timestamp.Before(sig.Timestamp) {
			matchTradeIdx++
		}
		if matchTradeIdx >= len(tradeCandles) {
			break
		}
		if !tradeCandles[matchTradeIdx].Timestamp.Equal(sig.Timestamp) {
			continue
		}
		earliestExecution := sig.Timestamp.Add(interval)
		if executionTradeIdx < matchTradeIdx {
			executionTradeIdx = matchTradeIdx
		}
		for executionTradeIdx < len(tradeCandles) && tradeCandles[executionTradeIdx].Timestamp.Before(earliestExecution) {
			executionTradeIdx++
		}
		if executionTradeIdx >= len(tradeCandles) {
			break
		}
		scheduled = append(scheduled, scheduledSignal{
			Signal:             sig,
			ExecutionTimestamp: tradeCandles[executionTradeIdx].Timestamp,
		})
	}
	return scheduled
}

func intervalDuration(interval string) (time.Duration, error) {
	switch interval {
	case models.CandleInterval1D:
		return 24 * time.Hour, nil
	case models.CandleInterval5M:
		return 5 * time.Minute, nil
	default:
		return 0, fmt.Errorf("unknown candle interval: %s", interval)
	}
}

// recordTrade appends a closed trade and updates the running equity and max-drawdown accumulators.
// capital is the user's starting capital and is used as the denominator when the running equity
// peak is still at zero (i.e. no profitable trade has been closed yet).
func recordTrade(result *models.BacktestResult, trade models.Trade, equity, peakEquity, maxDrawdown *float64, capital float64) {
	result.Trades = append(result.Trades, trade)
	*equity += trade.PnL
	if *equity > *peakEquity {
		*peakEquity = *equity
	}
	var dd float64
	if *peakEquity > 0 {
		dd = (*peakEquity - *equity) / *peakEquity
	} else if capital > 0 && *equity < 0 {
		// Equity has never gone positive; measure the drop from zero relative to capital.
		dd = -(*equity) / capital
	}
	if dd > *maxDrawdown {
		*maxDrawdown = dd
	}
}

// isReversal reports whether sig is in the opposite direction of the open trade.
func isReversal(openSide, sigSide models.OrderSide) bool {
	return (openSide == models.OrderSideBuy && sigSide == models.OrderSideSell) ||
		(openSide == models.OrderSideSell && sigSide == models.OrderSideBuy)
}

// checkExitConditions tests target, stop-loss, and time-exit rules in that order.
// Returns the exit reason string and the exact exit price, or ("", 0) if no condition is met.
// For longs, target/SL are checked against candle High/Low; shorts use the inverse.
func checkExitConditions(trade *models.Trade, candle *models.Candle, strategy *models.Strategy) (string, float64) {
	if strategy.TargetPct != nil {
		target := *strategy.TargetPct / 100
		if trade.Side == models.OrderSideBuy {
			if candle.High >= trade.EntryPrice*(1+target) {
				return ExitReasonTarget, trade.EntryPrice * (1 + target)
			}
		} else {
			if candle.Low <= trade.EntryPrice*(1-target) {
				return ExitReasonTarget, trade.EntryPrice * (1 - target)
			}
		}
	}

	if strategy.StopLossPct != nil {
		sl := *strategy.StopLossPct / 100
		if trade.Side == models.OrderSideBuy {
			if candle.Low <= trade.EntryPrice*(1-sl) {
				return ExitReasonStopLoss, trade.EntryPrice * (1 - sl)
			}
		} else {
			if candle.High >= trade.EntryPrice*(1+sl) {
				return ExitReasonStopLoss, trade.EntryPrice * (1 + sl)
			}
		}
	}

	if strategy.TimeExitMinutes != nil {
		elapsed := candle.Timestamp.Sub(trade.EntryTimestamp)
		if elapsed >= time.Duration(*strategy.TimeExitMinutes)*time.Minute {
			return ExitReasonTimeExit, candle.Close
		}
	}

	return "", 0
}

// closeTrade mutates trade in place with exit fields and computes PnL.
// PnL is positive for a profitable long (exitPrice > entryPrice) and
// for a profitable short (entryPrice > exitPrice).
func closeTrade(trade *models.Trade, exitPrice float64, exitTime time.Time, reason string, qty int) {
	trade.ExitTimestamp = exitTime
	trade.ExitPrice = exitPrice
	trade.ExitReason = reason
	if trade.Side == models.OrderSideBuy {
		trade.PnL = (exitPrice - trade.EntryPrice) * float64(qty)
	} else {
		trade.PnL = (trade.EntryPrice - exitPrice) * float64(qty)
	}
}
