package engine

import (
	"math"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type Trade struct {
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	Side          string
	Quantity      int
	EntryPrice    float64
	ExitPrice     float64
	PnL           float64
	Reason        string
	ExitReason    string
}

type BacktestResult struct {
	Trades      []Trade
	NetPnL      float64
	TotalTrades int
	WinCount    int
	LossCount   int
	MaxDrawdown float64
}

func RunBacktest(strategy *models.Strategy, candles []models.Candle) (*BacktestResult, error) {
	signals, err := Evaluate(strategy, candles)
	if err != nil {
		return nil, err
	}

	qty := strategy.LotSize
	if qty <= 0 {
		qty = 1
	}

	result := &BacktestResult{}
	var openTrade *Trade
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
				if (openTrade.Side == "BUY" && sig.Side == SignalSell) ||
					(openTrade.Side == "SELL" && sig.Side == SignalBuy) {
					closeTrade(openTrade, sig.Price, sig.Timestamp, "signal_reversal", qty)
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

			openTrade = &Trade{
				EntryTimestamp: sig.Timestamp,
				Side:          string(sig.Side),
				Quantity:      qty,
				EntryPrice:    sig.Price,
				Reason:        sig.Reason,
			}
		}
	}

	if openTrade != nil {
		last := candles[len(candles)-1]
		closeTrade(openTrade, last.Close, last.Timestamp, "end_of_data", qty)
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

func checkExitConditions(trade *Trade, candle *models.Candle, strategy *models.Strategy) (string, float64) {
	if strategy.TargetPct != nil {
		target := *strategy.TargetPct / 100
		if trade.Side == "BUY" {
			if candle.High >= trade.EntryPrice*(1+target) {
				return "target", trade.EntryPrice * (1 + target)
			}
		} else {
			if candle.Low <= trade.EntryPrice*(1-target) {
				return "target", trade.EntryPrice * (1 - target)
			}
		}
	}

	if strategy.StopLossPct != nil {
		sl := *strategy.StopLossPct / 100
		if trade.Side == "BUY" {
			if candle.Low <= trade.EntryPrice*(1-sl) {
				return "stop_loss", trade.EntryPrice * (1 - sl)
			}
		} else {
			if candle.High >= trade.EntryPrice*(1+sl) {
				return "stop_loss", trade.EntryPrice * (1 + sl)
			}
		}
	}

	if strategy.TimeExitMinutes != nil {
		elapsed := candle.Timestamp.Sub(trade.EntryTimestamp)
		if elapsed >= time.Duration(*strategy.TimeExitMinutes)*time.Minute {
			return "time_exit", candle.Close
		}
	}

	return "", 0
}

func closeTrade(trade *Trade, exitPrice float64, exitTime time.Time, reason string, qty int) {
	trade.ExitTimestamp = exitTime
	trade.ExitPrice = exitPrice
	trade.ExitReason = reason
	if trade.Side == "BUY" {
		trade.PnL = (exitPrice - trade.EntryPrice) * float64(qty)
	} else {
		trade.PnL = (trade.EntryPrice - exitPrice) * float64(qty)
	}
}
