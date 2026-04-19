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

type Backtester struct{}

func NewBacktester() *Backtester {
	return &Backtester{}
}

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
	equity := 0.0
	peakEquity := 0.0
	maxDrawdown := 0.0

	for i := range candles {
		if openTrade != nil {
			exitReason, exitPrice := checkExitConditions(openTrade, &candles[i], strategy)
			if exitReason != "" {
				closeTrade(openTrade, exitPrice, candles[i].Timestamp, exitReason, qty)
				result.Trades = append(result.Trades, *openTrade)
				equity += openTrade.PnL
				if equity > peakEquity {
					peakEquity = equity
				}
				if peakEquity > 0 {
					dd := (peakEquity - equity) / peakEquity
					if dd > maxDrawdown {
						maxDrawdown = dd
					}
				}
				openTrade = nil
			}
		}

		for sigIdx < len(signals) && signals[sigIdx].Timestamp.Equal(candles[i].Timestamp) {
			sig := signals[sigIdx]
			sigIdx++

			if openTrade != nil {
				if (openTrade.Side == models.OrderSideBuy && sig.Side == models.OrderSideSell) ||
					(openTrade.Side == models.OrderSideSell && sig.Side == models.OrderSideBuy) {
					closeTrade(openTrade, sig.Price, sig.Timestamp, ExitReasonSignalReversal, qty)
					result.Trades = append(result.Trades, *openTrade)
					equity += openTrade.PnL
					if equity > peakEquity {
						peakEquity = equity
					}
					if peakEquity > 0 {
						dd := (peakEquity - equity) / peakEquity
						if dd > maxDrawdown {
							maxDrawdown = dd
						}
					}
					openTrade = nil
				} else {
					continue
				}
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

	if openTrade != nil {
		last := candles[len(candles)-1]
		closeTrade(openTrade, last.Close, last.Timestamp, ExitReasonEndOfData, qty)
		result.Trades = append(result.Trades, *openTrade)
		equity += openTrade.PnL
		if equity > peakEquity {
			peakEquity = equity
		}
		if peakEquity > 0 {
			dd := (peakEquity - equity) / peakEquity
			if dd > maxDrawdown {
				maxDrawdown = dd
			}
		}
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
