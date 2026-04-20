package engine

import (
	"math"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// SMA returns a slice of Simple Moving Average values aligned to closes.
// The first (period-1) values are NaN (insufficient data).
func SMA(closes []float64, period int) []float64 {
	n := len(closes)
	if period <= 0 || n == 0 {
		return nanSlice(n)
	}

	out := make([]float64, n)
	for i := 0; i < period-1 && i < n; i++ {
		out[i] = math.NaN()
	}

	if period > n {
		return out
	}

	sum := 0.0
	for i := range period {
		sum += closes[i]
	}
	out[period-1] = sum / float64(period)

	for i := period; i < n; i++ {
		sum += closes[i] - closes[i-period]
		out[i] = sum / float64(period)
	}

	return out
}

// EMA returns a slice of Exponential Moving Average values aligned to closes.
// Seeded from the first SMA(period), then applies the standard 2/(period+1) smoothing factor.
// The first (period-1) values are NaN.
func EMA(closes []float64, period int) []float64 {
	n := len(closes)
	if period <= 0 || n == 0 {
		return nanSlice(n)
	}

	out := make([]float64, n)
	for i := 0; i < period-1 && i < n; i++ {
		out[i] = math.NaN()
	}

	if period > n {
		return out
	}

	sum := 0.0
	for i := range period {
		sum += closes[i]
	}
	seed := sum / float64(period)
	out[period-1] = seed

	k := 2.0 / float64(period+1)
	for i := period; i < n; i++ {
		out[i] = closes[i]*k + out[i-1]*(1-k)
	}

	return out
}

// RSI returns a slice of Relative Strength Index values aligned to closes.
// Uses Wilder's smoothed moving average. The first (period) values are NaN.
// RSI is clamped to 100 when average loss is zero.
func RSI(closes []float64, period int) []float64 {
	n := len(closes)
	if period <= 0 || n == 0 {
		return nanSlice(n)
	}

	out := make([]float64, n)
	for i := 0; i <= period && i < n; i++ {
		out[i] = math.NaN()
	}

	if n < period+1 {
		return out
	}

	gains := make([]float64, n)
	losses := make([]float64, n)
	for i := 1; i < n; i++ {
		delta := closes[i] - closes[i-1]
		if delta > 0 {
			gains[i] = delta
		} else {
			losses[i] = -delta
		}
	}

	sumGain := 0.0
	sumLoss := 0.0
	for i := 1; i <= period; i++ {
		sumGain += gains[i]
		sumLoss += losses[i]
	}
	avgGain := sumGain / float64(period)
	avgLoss := sumLoss / float64(period)

	out[period] = rsiValue(avgGain, avgLoss)

	for i := period + 1; i < n; i++ {
		avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)
		out[i] = rsiValue(avgGain, avgLoss)
	}

	return out
}

// ATR returns a slice of Average True Range values aligned to candles.
// True range is max(H-L, |H-prevC|, |L-prevC|). Smoothed with Wilder's method.
// The first (period-1) values are NaN.
func ATR(candles []models.Candle, period int) []float64 {
	n := len(candles)
	if period <= 0 || n == 0 {
		return nanSlice(n)
	}

	tr := make([]float64, n)
	tr[0] = candles[0].High - candles[0].Low
	for i := 1; i < n; i++ {
		hl := candles[i].High - candles[i].Low
		hpc := math.Abs(candles[i].High - candles[i-1].Close)
		lpc := math.Abs(candles[i].Low - candles[i-1].Close)
		tr[i] = math.Max(hl, math.Max(hpc, lpc))
	}

	out := make([]float64, n)
	for i := 0; i < period && i < n; i++ {
		out[i] = math.NaN()
	}

	if period > n {
		return out
	}

	sum := 0.0
	for i := range period {
		sum += tr[i]
	}
	out[period-1] = sum / float64(period)

	for i := period; i < n; i++ {
		out[i] = (out[i-1]*float64(period-1) + tr[i]) / float64(period)
	}

	return out
}

// Supertrend returns (st, dir) slices aligned to candles.
// st is the Supertrend line value; dir is +1 (bullish) or -1 (bearish).
// Band values are computed from ATR(period)*multiplier around the HL/2 midpoint.
// The first (period-1) entries are NaN/zero while ATR seeds.
func Supertrend(candles []models.Candle, period int, multiplier float64) ([]float64, []int) {
	n := len(candles)
	if period <= 0 || n == 0 {
		return nanSlice(n), make([]int, n)
	}

	atr := ATR(candles, period)

	st := make([]float64, n)
	dir := make([]int, n)
	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)

	for i := range n {
		if math.IsNaN(atr[i]) {
			upperBand[i] = math.NaN()
			lowerBand[i] = math.NaN()
			st[i] = math.NaN()
			dir[i] = 1
			continue
		}

		hl2 := (candles[i].High + candles[i].Low) / 2
		basicUpper := hl2 + multiplier*atr[i]
		basicLower := hl2 - multiplier*atr[i]

		if i > 0 && !math.IsNaN(upperBand[i-1]) {
			if basicUpper < upperBand[i-1] || candles[i-1].Close > upperBand[i-1] {
				upperBand[i] = basicUpper
			} else {
				upperBand[i] = upperBand[i-1]
			}

			if basicLower > lowerBand[i-1] || candles[i-1].Close < lowerBand[i-1] {
				lowerBand[i] = basicLower
			} else {
				lowerBand[i] = lowerBand[i-1]
			}
		} else {
			upperBand[i] = basicUpper
			lowerBand[i] = basicLower
		}

		if i == 0 || math.IsNaN(st[i-1]) {
			if candles[i].Close <= upperBand[i] {
				dir[i] = 1
				st[i] = lowerBand[i]
			} else {
				dir[i] = -1
				st[i] = upperBand[i]
			}
			continue
		}

		prevDir := dir[i-1]
		if prevDir == 1 {
			if candles[i].Close < lowerBand[i] {
				dir[i] = -1
				st[i] = upperBand[i]
			} else {
				dir[i] = 1
				st[i] = lowerBand[i]
			}
		} else {
			if candles[i].Close > upperBand[i] {
				dir[i] = 1
				st[i] = lowerBand[i]
			} else {
				dir[i] = -1
				st[i] = upperBand[i]
			}
		}
	}

	return st, dir
}

// nanSlice returns a length-n slice filled with NaN. Used to mark positions
// where an indicator has insufficient data rather than returning a misleading zero.
func nanSlice(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

// rsiValue computes an RSI reading from pre-smoothed average gain and loss.
// Returns 100 when avgLoss is zero (no losing periods in the window).
func rsiValue(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}
