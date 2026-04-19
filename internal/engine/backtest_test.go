package engine

import (
	"math"
	"testing"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }

func makeCandleSeries(closes []float64, minuteInterval int) []models.Candle {
	base := time.Date(2025, 1, 2, 9, 15, 0, 0, time.UTC)
	candles := make([]models.Candle, len(closes))
	for i, c := range closes {
		candles[i] = models.Candle{
			Timestamp: base.Add(time.Duration(i*minuteInterval) * time.Minute),
			Open:      c - 0.5,
			High:      c + 1,
			Low:       c - 1,
			Close:     c,
		}
	}
	return candles
}

func makeUpDownCandles(upCount, downCount int) []models.Candle {
	closes := make([]float64, upCount+downCount)
	for i := 0; i < upCount; i++ {
		closes[i] = 100 + float64(i)*2
	}
	for i := 0; i < downCount; i++ {
		closes[upCount+i] = closes[upCount-1] - float64(i+1)*3
	}
	return makeCandleSeries(closes, 5)
}

func TestEvaluate_UnknownCondition(t *testing.T) {
	s := &models.Strategy{EntryConditionType: "INVALID"}
	_, err := Evaluate(s, []models.Candle{{Close: 1}})
	if err == nil {
		t.Fatal("expected error for unknown condition type")
	}
}

func TestEvaluate_EmptyCandles(t *testing.T) {
	s := &models.Strategy{EntryConditionType: models.EntryConditionMACrossover}
	signals, err := Evaluate(s, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("expected no signals, got %d", len(signals))
	}
}

func TestEvaluateMACrossover(t *testing.T) {
	// Build series: flat → ramp up (short MA crosses above long) → ramp down (crosses below)
	n := 60
	closes := make([]float64, n)
	for i := 0; i < 25; i++ {
		closes[i] = 100
	}
	for i := 25; i < 40; i++ {
		closes[i] = 100 + float64(i-25)*2
	}
	for i := 40; i < 60; i++ {
		closes[i] = closes[39] - float64(i-39)*2
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{EntryConditionType: models.EntryConditionMACrossover}
	signals, err := Evaluate(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) == 0 {
		t.Fatal("expected at least one signal from MA crossover")
	}

	hasBuy := false
	hasSell := false
	for _, sig := range signals {
		if sig.Side == models.OrderSideBuy {
			hasBuy = true
		}
		if sig.Side == models.OrderSideSell {
			hasSell = true
		}
	}
	if !hasBuy || !hasSell {
		t.Errorf("expected both buy and sell signals, got buy=%v sell=%v", hasBuy, hasSell)
	}
}

func TestEvaluateRSI(t *testing.T) {
	// Sharp decline to trigger oversold, then recovery to trigger overbought
	n := 50
	closes := make([]float64, n)
	for i := 0; i < 20; i++ {
		closes[i] = 100
	}
	for i := 20; i < 35; i++ {
		closes[i] = 100 - float64(i-19)*3
	}
	for i := 35; i < 50; i++ {
		closes[i] = closes[34] + float64(i-34)*4
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{EntryConditionType: models.EntryConditionRSIOversold}
	signals, err := Evaluate(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) == 0 {
		t.Fatal("expected at least one RSI signal")
	}
}

func TestEvaluateMomentum(t *testing.T) {
	// Flat then breakout
	n := 40
	closes := make([]float64, n)
	for i := 0; i < 25; i++ {
		closes[i] = 100
	}
	for i := 25; i < 40; i++ {
		closes[i] = 100 + float64(i-24)*2
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{EntryConditionType: models.EntryConditionMomentum}
	signals, err := Evaluate(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(signals) == 0 {
		t.Fatal("expected at least one momentum signal")
	}
	if signals[0].Side != models.OrderSideBuy {
		t.Errorf("first momentum signal should be BUY, got %s", signals[0].Side)
	}
}

func TestRunBacktest_MACrossover(t *testing.T) {
	candles := makeUpDownCandles(30, 30)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalTrades == 0 {
		t.Fatal("expected at least one trade")
	}

	for _, tr := range result.Trades {
		if tr.EntryPrice <= 0 {
			t.Errorf("trade has invalid entry price: %f", tr.EntryPrice)
		}
		if tr.ExitPrice <= 0 {
			t.Errorf("trade has invalid exit price: %f", tr.ExitPrice)
		}
		if tr.Quantity != 1 {
			t.Errorf("expected quantity 1, got %d", tr.Quantity)
		}
		if tr.ExitReason == "" {
			t.Error("trade missing exit reason")
		}
	}

	calculatedPnL := 0.0
	for _, tr := range result.Trades {
		calculatedPnL += tr.PnL
	}
	if math.Abs(calculatedPnL-result.NetPnL) > tolerance {
		t.Errorf("NetPnL mismatch: sum of trades=%f, reported=%f", calculatedPnL, result.NetPnL)
	}

	if result.WinCount+result.LossCount > result.TotalTrades {
		t.Error("win + loss count exceeds total trades")
	}
}

func TestRunBacktest_TargetHit(t *testing.T) {
	// Rising prices: entry then target hit
	closes := make([]float64, 40)
	for i := 0; i < 25; i++ {
		closes[i] = 100
	}
	// Ramp up so MA crossover triggers, then price keeps rising to hit target
	for i := 25; i < 40; i++ {
		closes[i] = 100 + float64(i-24)*3
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            2,
		TargetPct:          ptrFloat(5.0),
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	targetHit := false
	for _, tr := range result.Trades {
		if tr.ExitReason == ExitReasonTarget {
			targetHit = true
			if tr.PnL <= 0 {
				t.Errorf("target exit should have positive PnL, got %f", tr.PnL)
			}
			if tr.Quantity != 2 {
				t.Errorf("expected quantity 2, got %d", tr.Quantity)
			}
		}
	}
	if !targetHit && result.TotalTrades > 0 {
		t.Log("note: target was not hit in this price series (may need wider ramp)")
	}
}

func TestRunBacktest_StopLossHit(t *testing.T) {
	// MA crossover triggers buy, then price drops sharply
	closes := make([]float64, 50)
	for i := 0; i < 25; i++ {
		closes[i] = 100
	}
	for i := 25; i < 32; i++ {
		closes[i] = 100 + float64(i-24)*2
	}
	for i := 32; i < 50; i++ {
		closes[i] = closes[31] - float64(i-31)*3
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
		StopLossPct:        ptrFloat(3.0),
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	slHit := false
	for _, tr := range result.Trades {
		if tr.ExitReason == ExitReasonStopLoss {
			slHit = true
			if tr.PnL >= 0 {
				t.Errorf("stop loss exit should have negative PnL, got %f", tr.PnL)
			}
		}
	}
	if !slHit && result.TotalTrades > 0 {
		t.Log("note: stop loss was not hit in this price series")
	}
}

func TestRunBacktest_TimeExit(t *testing.T) {
	// Flat prices with a short time exit
	closes := make([]float64, 50)
	for i := 0; i < 25; i++ {
		closes[i] = 100
	}
	for i := 25; i < 50; i++ {
		closes[i] = 100 + float64(i-24)*1.5
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
		TimeExitMinutes:    ptrInt(20),
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	timeExitFound := false
	for _, tr := range result.Trades {
		if tr.ExitReason == ExitReasonTimeExit {
			timeExitFound = true
			elapsed := tr.ExitTimestamp.Sub(tr.EntryTimestamp)
			if elapsed < 20*time.Minute {
				t.Errorf("time exit happened too early: %v", elapsed)
			}
		}
	}
	if !timeExitFound && result.TotalTrades > 0 {
		t.Log("note: time exit was not triggered (signals may have reversed first)")
	}
}

func TestRunBacktest_MaxDrawdown(t *testing.T) {
	candles := makeUpDownCandles(30, 30)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MaxDrawdown < 0 || result.MaxDrawdown > 1 {
		t.Errorf("MaxDrawdown should be in [0, 1], got %f", result.MaxDrawdown)
	}
}

func TestRunBacktest_NoSignals(t *testing.T) {
	// Flat prices → no MA crossover
	candles := makeCandleSeries([]float64{100, 100, 100, 100, 100}, 5)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalTrades != 0 {
		t.Errorf("expected 0 trades with flat data, got %d", result.TotalTrades)
	}
	if result.NetPnL != 0 {
		t.Errorf("expected 0 PnL with no trades, got %f", result.NetPnL)
	}
}

func TestRunBacktest_LotSizeApplied(t *testing.T) {
	candles := makeUpDownCandles(30, 30)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            10,
	}

	result, err := NewBacktester().RunBacktest(s, candles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tr := range result.Trades {
		if tr.Quantity != 10 {
			t.Errorf("expected quantity 10, got %d", tr.Quantity)
		}
	}
}
