package engine

import (
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

type Backtester struct{}

func NewBacktester() *Backtester {
	return &Backtester{}
}

// RunBacktest simulates a strategy against historical candles.
// Each candle is processed in order:
//  1. Exit the open trade if target/stop-loss/time-exit is hit.
//  2. Process any signals at the same timestamp — reversals close + reopen, same-direction signals are ignored.
//
// Any trade still open at end of data is closed at the last candle's close price.
func (b *Backtester) RunBacktest(strategy *models.Strategy, candles []models.Candle) (*models.BacktestResult, error) {
	signals, err := Evaluate(strategy, candles)
	if err != nil {
		return nil, err
	}

	qty := strategy.LotSize
	if qty <= 0 {
		qty = 1
	}

	result := &models.BacktestResult{}
	var openTrade *models.Trade
	sigIdx := 0
	equity, peakEquity, maxDrawdown := 0.0, 0.0, 0.0

	for i := range candles {
		candle := &candles[i]

		// Exit takes priority: check before processing new signals at the same timestamp.
		if openTrade != nil {
			if reason, price := checkExitConditions(openTrade, candle, strategy); reason != "" {
				closeTrade(openTrade, price, candle.Timestamp, reason, qty)
				recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown)
				openTrade = nil
			}
		}

		// Walk signals that align to this candle's timestamp (there can be more than one).
		for sigIdx < len(signals) && signals[sigIdx].Timestamp.Equal(candle.Timestamp) {
			sig := signals[sigIdx]
			sigIdx++

			if openTrade != nil {
				if !isReversal(openTrade.Side, sig.Side) {
					// Same-direction signal while already in a trade — skip, no pyramiding.
					continue
				}
				// Opposite-direction signal: close current trade then fall through to open a new one.
				closeTrade(openTrade, sig.Price, sig.Timestamp, ExitReasonSignalReversal, qty)
				recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown)
				openTrade = nil
			}

			openTrade = &models.Trade{
				EntryTimestamp: sig.Timestamp,
				Side:           sig.Side,
				Quantity:       qty,
				EntryPrice:     sig.Price,
				Reason:         sig.Reason,
			}
		}
	}

	// Close any trade still open at end of data.
	if openTrade != nil {
		last := candles[len(candles)-1]
		closeTrade(openTrade, last.Close, last.Timestamp, ExitReasonEndOfData, qty)
		recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown)
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

// recordTrade appends a closed trade and updates the running equity and max-drawdown accumulators.
func recordTrade(result *models.BacktestResult, trade models.Trade, equity, peakEquity, maxDrawdown *float64) {
	result.Trades = append(result.Trades, trade)
	*equity += trade.PnL
	if *equity > *peakEquity {
		*peakEquity = *equity
	}
	if *peakEquity > 0 {
		dd := (*peakEquity - *equity) / *peakEquity
		if dd > *maxDrawdown {
			*maxDrawdown = dd
		}
	}
}

// isReversal reports whether sig is in the opposite direction of the open trade.
func isReversal(openSide, sigSide models.OrderSide) bool {
	return (openSide == models.OrderSideBuy && sigSide == models.OrderSideSell) ||
		(openSide == models.OrderSideSell && sigSide == models.OrderSideBuy)
}

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
