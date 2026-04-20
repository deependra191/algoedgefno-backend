package engine

import (
	"math"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const tolerance = 1e-6

func assertClose(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.IsNaN(want) {
		if !math.IsNaN(got) {
			t.Errorf("%s: got %f, want NaN", label, got)
		}
		return
	}
	if math.IsNaN(got) {
		t.Errorf("%s: got NaN, want %f", label, want)
		return
	}
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s: got %f, want %f", label, got, want)
	}
}

func TestSMA_Basic(t *testing.T) {
	closes := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	result := SMA(closes, 3)

	if len(result) != len(closes) {
		t.Fatalf("length mismatch: got %d, want %d", len(result), len(closes))
	}

	assertClose(t, result[0], math.NaN(), "SMA[0]")
	assertClose(t, result[1], math.NaN(), "SMA[1]")
	assertClose(t, result[2], 11.0, "SMA[2]")
	assertClose(t, result[3], 12.0, "SMA[3]")
	assertClose(t, result[4], 13.0, "SMA[4]")
	assertClose(t, result[9], 18.0, "SMA[9]")
}

func TestSMA_PeriodOne(t *testing.T) {
	closes := []float64{5, 10, 15}
	result := SMA(closes, 1)

	assertClose(t, result[0], 5, "SMA[0]")
	assertClose(t, result[1], 10, "SMA[1]")
	assertClose(t, result[2], 15, "SMA[2]")
}

func TestSMA_PeriodEqualsLength(t *testing.T) {
	closes := []float64{2, 4, 6}
	result := SMA(closes, 3)

	assertClose(t, result[0], math.NaN(), "SMA[0]")
	assertClose(t, result[1], math.NaN(), "SMA[1]")
	assertClose(t, result[2], 4.0, "SMA[2]")
}

func TestSMA_PeriodExceedsLength(t *testing.T) {
	closes := []float64{1, 2}
	result := SMA(closes, 5)

	for i, v := range result {
		if !math.IsNaN(v) {
			t.Errorf("SMA[%d]: got %f, want NaN", i, v)
		}
	}
}

func TestSMA_Empty(t *testing.T) {
	result := SMA([]float64{}, 3)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestEMA_Basic(t *testing.T) {
	closes := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	result := EMA(closes, 3)

	if len(result) != len(closes) {
		t.Fatalf("length mismatch: got %d, want %d", len(result), len(closes))
	}

	assertClose(t, result[0], math.NaN(), "EMA[0]")
	assertClose(t, result[1], math.NaN(), "EMA[1]")

	// Seed = SMA(10,11,12) = 11.0
	assertClose(t, result[2], 11.0, "EMA[2] seed")

	// k = 2/(3+1) = 0.5
	// EMA[3] = 13*0.5 + 11.0*0.5 = 12.0
	assertClose(t, result[3], 12.0, "EMA[3]")
	// EMA[4] = 14*0.5 + 12.0*0.5 = 13.0
	assertClose(t, result[4], 13.0, "EMA[4]")
}

func TestEMA_PeriodExceedsLength(t *testing.T) {
	closes := []float64{1, 2}
	result := EMA(closes, 5)

	for i, v := range result {
		if !math.IsNaN(v) {
			t.Errorf("EMA[%d]: got %f, want NaN", i, v)
		}
	}
}

func TestEMA_Empty(t *testing.T) {
	result := EMA([]float64{}, 3)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestRSI_Basic(t *testing.T) {
	// 15 data points → first RSI at index 14 (period=14)
	closes := []float64{
		44, 44.34, 44.09, 44.15, 43.61,
		44.33, 44.83, 45.10, 45.42, 45.84,
		46.08, 45.89, 46.03, 45.61, 46.28,
	}
	result := RSI(closes, 14)

	if len(result) != len(closes) {
		t.Fatalf("length mismatch: got %d, want %d", len(result), len(closes))
	}

	for i := 0; i <= 13; i++ {
		if !math.IsNaN(result[i]) {
			t.Errorf("RSI[%d]: got %f, want NaN", i, result[i])
		}
	}

	// RSI[14] should be a valid value between 0 and 100
	if math.IsNaN(result[14]) || result[14] < 0 || result[14] > 100 {
		t.Errorf("RSI[14]: got %f, want value in [0, 100]", result[14])
	}
}

func TestRSI_AllGains(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := RSI(closes, 5)

	// With all gains and zero losses, RSI should be 100
	for i := 5; i < len(result); i++ {
		assertClose(t, result[i], 100.0, "RSI all gains")
	}
}

func TestRSI_AllLosses(t *testing.T) {
	closes := []float64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	result := RSI(closes, 5)

	// With all losses and zero gains, RSI should be 0
	for i := 5; i < len(result); i++ {
		assertClose(t, result[i], 0.0, "RSI all losses")
	}
}

func TestRSI_PeriodExceedsLength(t *testing.T) {
	closes := []float64{1, 2, 3}
	result := RSI(closes, 14)

	for i, v := range result {
		if !math.IsNaN(v) {
			t.Errorf("RSI[%d]: got %f, want NaN", i, v)
		}
	}
}

func TestRSI_Empty(t *testing.T) {
	result := RSI([]float64{}, 14)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func testCandles() []models.Candle {
	return []models.Candle{
		{High: 48.70, Low: 47.79, Close: 48.16},
		{High: 48.72, Low: 48.14, Close: 48.61},
		{High: 48.90, Low: 48.39, Close: 48.75},
		{High: 48.87, Low: 48.37, Close: 48.63},
		{High: 48.82, Low: 48.24, Close: 48.74},
		{High: 49.05, Low: 48.64, Close: 49.03},
		{High: 49.20, Low: 48.94, Close: 49.07},
		{High: 49.35, Low: 48.86, Close: 49.32},
		{High: 49.92, Low: 49.50, Close: 49.91},
		{High: 50.19, Low: 49.87, Close: 50.13},
		{High: 50.12, Low: 49.20, Close: 49.53},
		{High: 49.66, Low: 48.90, Close: 49.50},
		{High: 49.88, Low: 49.43, Close: 49.75},
		{High: 50.19, Low: 49.73, Close: 50.03},
		{High: 50.36, Low: 49.26, Close: 50.31},
		{High: 50.57, Low: 50.09, Close: 50.52},
		{High: 50.65, Low: 50.30, Close: 50.41},
		{High: 50.43, Low: 49.21, Close: 49.34},
		{High: 49.63, Low: 48.98, Close: 49.37},
		{High: 50.33, Low: 49.61, Close: 50.23},
	}
}

func TestATR_Basic(t *testing.T) {
	candles := testCandles()
	result := ATR(candles, 14)

	if len(result) != len(candles) {
		t.Fatalf("length mismatch: got %d, want %d", len(result), len(candles))
	}

	for i := 0; i < 13; i++ {
		if !math.IsNaN(result[i]) {
			t.Errorf("ATR[%d]: got %f, want NaN", i, result[i])
		}
	}

	// ATR[13] is the first valid value (SMA of first 14 true ranges)
	if math.IsNaN(result[13]) || result[13] <= 0 {
		t.Errorf("ATR[13]: got %f, want positive value", result[13])
	}

	// Subsequent ATR values should also be positive
	for i := 14; i < len(result); i++ {
		if math.IsNaN(result[i]) || result[i] <= 0 {
			t.Errorf("ATR[%d]: got %f, want positive value", i, result[i])
		}
	}
}

func TestATR_Empty(t *testing.T) {
	result := ATR([]models.Candle{}, 14)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestATR_TrueRangeUsesGaps(t *testing.T) {
	candles := []models.Candle{
		{High: 10, Low: 9, Close: 9.5},
		{High: 12, Low: 11, Close: 11.5}, // gap up: TR = max(1, |12-9.5|, |11-9.5|) = 2.5
	}
	result := ATR(candles, 1)

	// ATR period=1 means ATR = TR at each bar
	assertClose(t, result[0], 1.0, "ATR[0]") // first bar TR = high-low = 1
	// Second bar: Wilder smoothing with period 1 → just TR
	// TR = max(12-11, |12-9.5|, |11-9.5|) = max(1, 2.5, 1.5) = 2.5
	assertClose(t, result[1], 2.5, "ATR[1]")
}

func TestSupertrend_Basic(t *testing.T) {
	candles := testCandles()
	st, dir := Supertrend(candles, 10, 3.0)

	if len(st) != len(candles) || len(dir) != len(candles) {
		t.Fatalf("length mismatch: st=%d, dir=%d, want %d", len(st), len(dir), len(candles))
	}

	// First period-1 values should be NaN (ATR not yet available)
	for i := 0; i < 9; i++ {
		if !math.IsNaN(st[i]) {
			t.Errorf("Supertrend[%d]: got %f, want NaN", i, st[i])
		}
	}

	// After warmup, values should be valid
	for i := 9; i < len(candles); i++ {
		if math.IsNaN(st[i]) {
			t.Errorf("Supertrend[%d]: got NaN, want valid value", i)
		}
		if dir[i] != 1 && dir[i] != -1 {
			t.Errorf("Direction[%d]: got %d, want 1 or -1", i, dir[i])
		}
	}
}

func TestSupertrend_DirectionFlips(t *testing.T) {
	// Construct data with a clear trend reversal
	candles := make([]models.Candle, 30)
	// Uptrend for first 15 bars
	for i := 0; i < 15; i++ {
		base := 100.0 + float64(i)*2
		candles[i] = models.Candle{
			High:  base + 1,
			Low:   base - 1,
			Close: base,
		}
	}
	// Sharp downtrend for next 15 bars
	for i := 15; i < 30; i++ {
		base := 130.0 - float64(i-15)*3
		candles[i] = models.Candle{
			High:  base + 1,
			Low:   base - 1,
			Close: base,
		}
	}

	_, dir := Supertrend(candles, 7, 2.0)

	seenBullish := false
	seenBearish := false
	for i := 7; i < len(dir); i++ {
		if dir[i] == 1 {
			seenBullish = true
		}
		if dir[i] == -1 {
			seenBearish = true
		}
	}

	if !seenBullish || !seenBearish {
		t.Errorf("expected direction to flip: seenBullish=%v, seenBearish=%v", seenBullish, seenBearish)
	}
}

func TestSupertrend_Empty(t *testing.T) {
	st, dir := Supertrend([]models.Candle{}, 10, 3.0)
	if len(st) != 0 || len(dir) != 0 {
		t.Errorf("expected empty slices, got st=%d, dir=%d", len(st), len(dir))
	}
}

func TestSupertrend_BullishMeansLowerBand(t *testing.T) {
	candles := testCandles()
	st, dir := Supertrend(candles, 10, 3.0)

	for i := 9; i < len(candles); i++ {
		hl2 := (candles[i].High + candles[i].Low) / 2
		if dir[i] == 1 {
			if st[i] > hl2 {
				t.Errorf("Supertrend[%d]: bullish but ST=%f > hl2=%f", i, st[i], hl2)
			}
		} else {
			if st[i] < hl2 {
				t.Errorf("Supertrend[%d]: bearish but ST=%f < hl2=%f", i, st[i], hl2)
			}
		}
	}
}
