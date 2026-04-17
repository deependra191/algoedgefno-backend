package engine

import (
	"math"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func SMA(closes []float64, period int) []float64 {
	n := len(closes)
	out := make([]float64, n)

	if period <= 0 || n == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	for i := 0; i < period-1 && i < n; i++ {
		out[i] = math.NaN()
	}

	if period > n {
		return out
	}

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += closes[i]
	}
	out[period-1] = sum / float64(period)

	for i := period; i < n; i++ {
		sum += closes[i] - closes[i-period]
		out[i] = sum / float64(period)
	}

	return out
}

func EMA(closes []float64, period int) []float64 {
	n := len(closes)
	out := make([]float64, n)

	if period <= 0 || n == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	for i := 0; i < period-1 && i < n; i++ {
		out[i] = math.NaN()
	}

	if period > n {
		return out
	}

	sum := 0.0
	for i := 0; i < period; i++ {
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

func RSI(closes []float64, period int) []float64 {
	n := len(closes)
	out := make([]float64, n)

	if period <= 0 || n == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

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

	if avgLoss == 0 {
		out[period] = 100
	} else {
		rs := avgGain / avgLoss
		out[period] = 100 - (100 / (1 + rs))
	}

	for i := period + 1; i < n; i++ {
		avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)

		if avgLoss == 0 {
			out[i] = 100
		} else {
			rs := avgGain / avgLoss
			out[i] = 100 - (100 / (1 + rs))
		}
	}

	return out
}

func ATR(candles []models.Candle, period int) []float64 {
	n := len(candles)
	out := make([]float64, n)

	if period <= 0 || n == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	out[0] = math.NaN()

	tr := make([]float64, n)
	tr[0] = candles[0].High - candles[0].Low
	for i := 1; i < n; i++ {
		hl := candles[i].High - candles[i].Low
		hpc := math.Abs(candles[i].High - candles[i-1].Close)
		lpc := math.Abs(candles[i].Low - candles[i-1].Close)
		tr[i] = math.Max(hl, math.Max(hpc, lpc))
	}

	for i := 0; i < period && i < n; i++ {
		out[i] = math.NaN()
	}

	if period > n {
		return out
	}

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += tr[i]
	}
	out[period-1] = sum / float64(period)

	for i := period; i < n; i++ {
		out[i] = (out[i-1]*float64(period-1) + tr[i]) / float64(period)
	}

	return out
}

func Supertrend(candles []models.Candle, period int, multiplier float64) ([]float64, []int) {
	n := len(candles)
	st := make([]float64, n)
	dir := make([]int, n)

	if period <= 0 || n == 0 {
		for i := range st {
			st[i] = math.NaN()
		}
		return st, dir
	}

	atr := ATR(candles, period)

	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)

	for i := 0; i < n; i++ {
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
