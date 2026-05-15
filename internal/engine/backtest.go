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

	candleInterval1DDuration = 24 * time.Hour
	candleInterval5MDuration = 5 * time.Minute
	errMsgUnknownInterval    = "unknown candle interval"
)

var _ models.BacktestEngine = (*Backtester)(nil)

// Backtester is the standard implementation of models.BacktestEngine.
// It deducts a per-trade Indian retail cost stack (slippage, brokerage, statutory
// charges, GST, stamp) via the injected ChargeCalculator before computing NetPnL.
type Backtester struct {
	charges models.ChargeCalculator
}

// NewBacktester constructs a Backtester with the given ChargeCalculator.
// charges must be non-nil; closeTrade will panic on nil. The constructor stays
// strict by design — a silent zero-charge fallback would mask wiring bugs and
// corrupt P&L on every closed trade.
func NewBacktester(charges models.ChargeCalculator) *Backtester {
	return &Backtester{charges: charges}
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
	segment := strategy.InstrumentType

	for i := range inputs.TradeCandles {
		candle := &inputs.TradeCandles[i]

		if openTrade != nil {
			if reason, price := checkExitConditions(openTrade, candle, strategy); reason != "" {
				b.closeTrade(openTrade, price, candle.Timestamp, reason, qty, segment)
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
				b.closeTrade(openTrade, candle.Open, candle.Timestamp, ExitReasonSignalReversal, qty, segment)
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
		b.closeTrade(openTrade, last.Close, last.Timestamp, ExitReasonEndOfData, qty, segment)
		recordTrade(result, *openTrade, &equity, &peakEquity, &maxDrawdown, capital)
	}

	result.TotalTrades = len(result.Trades)
	for _, tr := range result.Trades {
		result.GrossPnL += tr.GrossPnL
		result.TotalCharges += tr.TotalCharges
		if tr.NetPnL > 0 {
			result.WinCount++
		} else if tr.NetPnL < 0 {
			result.LossCount++
		}
	}
	result.NetPnL = result.GrossPnL - result.TotalCharges
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
		return candleInterval1DDuration, nil
	case models.CandleInterval5M:
		return candleInterval5MDuration, nil
	default:
		return 0, fmt.Errorf("%s: %s", errMsgUnknownInterval, interval)
	}
}

// recordTrade appends a closed trade and updates the running equity and max-drawdown accumulators.
// capital is the user's starting capital and is used as the denominator when the running equity
// peak is still at zero (i.e. no profitable trade has been closed yet).
func recordTrade(result *models.BacktestResult, trade models.Trade, equity, peakEquity, maxDrawdown *float64, capital float64) {
	result.Trades = append(result.Trades, trade)
	*equity += trade.NetPnL
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

// closeTrade mutates trade in place with exit fields, the frictionless GrossPnL,
// the full charge breakdown from the injected ChargeCalculator, and the
// post-charges NetPnL. GrossPnL is positive for a profitable long (exitPrice >
// entryPrice) and for a profitable short (entryPrice > exitPrice); NetPnL =
// GrossPnL − TotalCharges.
func (b *Backtester) closeTrade(trade *models.Trade, exitPrice float64, exitTime time.Time, reason string, qty int, segment string) {
	trade.ExitTimestamp = exitTime
	trade.ExitPrice = exitPrice
	trade.ExitReason = reason
	if trade.Side == models.OrderSideBuy {
		trade.GrossPnL = (exitPrice - trade.EntryPrice) * float64(qty)
	} else {
		trade.GrossPnL = (trade.EntryPrice - exitPrice) * float64(qty)
	}

	cb := b.charges.Compute(segment, trade.Side, trade.EntryPrice, exitPrice, qty)
	trade.Slippage = cb.Slippage
	trade.Brokerage = cb.Brokerage
	trade.STT = cb.STT
	trade.ExchangeFees = cb.ExchangeFees
	trade.SEBIFees = cb.SEBIFees
	trade.GST = cb.GST
	trade.StampDuty = cb.StampDuty
	trade.TotalCharges = cb.Total
	trade.NetPnL = trade.GrossPnL - cb.Total
}
