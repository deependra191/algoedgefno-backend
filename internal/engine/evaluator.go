package engine

import (
	"fmt"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	defaultShortMA        = 9
	defaultLongMA         = 21
	defaultSupertrendPer  = 10
	defaultSupertrendMult = 3.0
	defaultRSIPeriod      = 14
	defaultRSIOversold    = 30.0
	defaultRSIOverbought  = 70.0
	defaultMomentumLookback = 20
)

// Evaluate generates entry signals for the given strategy by dispatching to the
// appropriate indicator-based evaluator. Candles must be in chronological order.
// Returns nil, nil for an empty candle slice.
func Evaluate(strategy *models.Strategy, candles []models.Candle) ([]Signal, error) {
	if len(candles) == 0 {
		return nil, nil
	}

	switch strategy.EntryConditionType {
	case models.EntryConditionMACrossover:
		return evaluateMACrossover(candles)
	case models.EntryConditionSupertrend:
		return evaluateSupertrend(candles)
	case models.EntryConditionRSIOversold:
		return evaluateRSI(candles)
	case models.EntryConditionMomentum:
		return evaluateMomentum(candles)
	default:
		return nil, fmt.Errorf("unknown entry condition type: %s", strategy.EntryConditionType)
	}
}

// evaluateMACrossover emits a BUY when the short MA crosses above the long MA,
// and a SELL on the reverse crossover.
func evaluateMACrossover(candles []models.Candle) ([]Signal, error) {
	closes := extractCloses(candles)
	shortMA := SMA(closes, defaultShortMA)
	longMA := SMA(closes, defaultLongMA)

	var signals []Signal
	for i := defaultLongMA; i < len(candles); i++ {
		prevShortAbove := shortMA[i-1] > longMA[i-1]
		currShortAbove := shortMA[i] > longMA[i]

		if currShortAbove && !prevShortAbove {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideBuy,
				Price:     candles[i].Close,
				Reason:    "MA crossover bullish",
			})
		} else if !currShortAbove && prevShortAbove {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideSell,
				Price:     candles[i].Close,
				Reason:    "MA crossover bearish",
			})
		}
	}
	return signals, nil
}

// evaluateSupertrend emits a BUY on a bearish→bullish Supertrend flip and a SELL on the reverse.
func evaluateSupertrend(candles []models.Candle) ([]Signal, error) {
	_, dir := Supertrend(candles, defaultSupertrendPer, defaultSupertrendMult)

	var signals []Signal
	for i := defaultSupertrendPer; i < len(candles); i++ {
		if dir[i] == 1 && dir[i-1] == -1 {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideBuy,
				Price:     candles[i].Close,
				Reason:    "Supertrend bullish flip",
			})
		} else if dir[i] == -1 && dir[i-1] == 1 {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideSell,
				Price:     candles[i].Close,
				Reason:    "Supertrend bearish flip",
			})
		}
	}
	return signals, nil
}

// evaluateRSI emits a BUY when RSI crosses below the oversold threshold
// and a SELL when it crosses above the overbought threshold.
func evaluateRSI(candles []models.Candle) ([]Signal, error) {
	closes := extractCloses(candles)
	rsi := RSI(closes, defaultRSIPeriod)

	var signals []Signal
	for i := defaultRSIPeriod + 1; i < len(candles); i++ {
		if rsi[i] < defaultRSIOversold && rsi[i-1] >= defaultRSIOversold {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideBuy,
				Price:     candles[i].Close,
				Reason:    "RSI crossed below oversold",
			})
		} else if rsi[i] > defaultRSIOverbought && rsi[i-1] <= defaultRSIOverbought {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideSell,
				Price:     candles[i].Close,
				Reason:    "RSI crossed above overbought",
			})
		}
	}
	return signals, nil
}

// evaluateMomentum emits a BUY when price closes above the N-period high (breakout)
// and a SELL when it falls back below. Only one position at a time — re-entry requires
// price to drop below the high and break out again.
func evaluateMomentum(candles []models.Candle) ([]Signal, error) {
	n := len(candles)
	if n <= defaultMomentumLookback {
		return nil, nil
	}

	var signals []Signal
	inBreakout := false

	for i := defaultMomentumLookback; i < n; i++ {
		periodHigh := 0.0
		for j := i - defaultMomentumLookback; j < i; j++ {
			if candles[j].High > periodHigh {
				periodHigh = candles[j].High
			}
		}

		if candles[i].Close > periodHigh && !inBreakout {
			signals = append(signals, Signal{
				Timestamp: candles[i].Timestamp,
				Side:      models.OrderSideBuy,
				Price:     candles[i].Close,
				Reason:    fmt.Sprintf("Breakout above %d-period high", defaultMomentumLookback),
			})
			inBreakout = true
		} else if candles[i].Close <= periodHigh {
			if inBreakout {
				signals = append(signals, Signal{
					Timestamp: candles[i].Timestamp,
					Side:      models.OrderSideSell,
					Price:     candles[i].Close,
					Reason:    fmt.Sprintf("Fell below %d-period high", defaultMomentumLookback),
				})
			}
			inBreakout = false
		}
	}
	return signals, nil
}

func extractCloses(candles []models.Candle) []float64 {
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	return closes
}
