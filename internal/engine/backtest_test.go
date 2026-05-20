package engine

import (
	"math"
	"testing"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }

const maxDrawdownTolerance = 1e-9

// zeroCharges is a test stub implementing models.ChargeCalculator that returns no
// charges. It keeps the legacy frictionless-P&L assertions in backtest_test.go
// valid; tests exercising real charges use engine.NewCharges() directly.
type zeroCharges struct{}

func (zeroCharges) Compute(string, models.OrderSide, float64, float64, int) models.ChargeBreakdown {
	return models.ChargeBreakdown{}
}

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

func makeMomentumSignalCandles() []models.Candle {
	closes := make([]float64, 21)
	for i := 0; i < 20; i++ {
		closes[i] = 100
	}
	closes[20] = 110
	return makeCandleSeries(closes, 5)
}

func sameStreamInputs(candles []models.Candle) models.EngineInputs {
	return models.EngineInputs{
		Interval:      models.CandleInterval5M,
		SignalCandles: candles,
		TradeCandles:  candles,
	}
}

func tradeCandlesAt(times []time.Time, opens []float64) []models.Candle {
	candles := make([]models.Candle, len(times))
	for i := range times {
		open := opens[i]
		candles[i] = models.Candle{
			Timestamp: times[i],
			Open:      open,
			High:      open + 1,
			Low:       open - 1,
			Close:     open + 0.5,
		}
	}
	return candles
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

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
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
		calculatedPnL += tr.NetPnL
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
		NumberOfLots:       1,
		TargetPct:          ptrFloat(5.0),
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	targetHit := false
	for _, tr := range result.Trades {
		if tr.ExitReason == ExitReasonTarget {
			targetHit = true
			if tr.NetPnL <= 0 {
				t.Errorf("target exit should have positive PnL, got %f", tr.NetPnL)
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

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	slHit := false
	for _, tr := range result.Trades {
		if tr.ExitReason == ExitReasonStopLoss {
			slHit = true
			if tr.NetPnL >= 0 {
				t.Errorf("stop loss exit should have negative PnL, got %f", tr.NetPnL)
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

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
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

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MaxDrawdown < 0 || result.MaxDrawdown > 1 {
		t.Errorf("MaxDrawdown should be in [0, 1], got %f", result.MaxDrawdown)
	}
}

func TestRecordTrade_MaxDrawdownUsesCapitalSeededRunningPeak(t *testing.T) {
	result := &models.BacktestResult{}
	equity := 100000.0
	peakEquity := equity
	maxDrawdown := 0.0

	for _, netPnL := range []float64{5000, -3000, -1000, 10000} {
		recordTrade(result, models.Trade{NetPnL: netPnL}, &equity, &peakEquity, &maxDrawdown)
	}

	expectedMaxDrawdown := 4000.0 / 105000.0
	if math.Abs(maxDrawdown-expectedMaxDrawdown) > maxDrawdownTolerance {
		t.Fatalf("expected max drawdown %.10f, got %.10f", expectedMaxDrawdown, maxDrawdown)
	}
	result.MaxDrawdown = math.Round(maxDrawdown*10000) / 10000
	if result.MaxDrawdown != 0.0381 {
		t.Fatalf("expected rounded BacktestResult.MaxDrawdown 0.0381, got %.4f", result.MaxDrawdown)
	}
	if peakEquity != 111000 {
		t.Fatalf("expected final peak equity 111000, got %.2f", peakEquity)
	}
	if equity != 111000 {
		t.Fatalf("expected final equity 111000, got %.2f", equity)
	}
}

func TestRunBacktest_NoSignals(t *testing.T) {
	// Flat prices → no MA crossover
	candles := makeCandleSeries([]float64{100, 100, 100, 100, 100}, 5)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            1,
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
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

func TestRunBacktest_QuantityIsLotSizeTimesNumberOfLots(t *testing.T) {
	candles := makeUpDownCandles(30, 30)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		LotSize:            50,
		NumberOfLots:       3,
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tr := range result.Trades {
		if tr.Quantity != 150 {
			t.Errorf("expected quantity 150 (50*3), got %d", tr.Quantity)
		}
	}
}

func TestRunBacktest_SplitStreamsExecuteAtNextTradeOpen(t *testing.T) {
	signalCandles := makeMomentumSignalCandles()
	signalTime := signalCandles[len(signalCandles)-1].Timestamp
	tradeCandles := tradeCandlesAt(
		[]time.Time{signalTime, signalTime.Add(5 * time.Minute), signalTime.Add(10 * time.Minute)},
		[]float64{200, 250, 260},
	)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMomentum,
		LotSize:            1,
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, models.EngineInputs{
		Interval:      models.CandleInterval5M,
		SignalCandles: signalCandles,
		TradeCandles:  tradeCandles,
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades != 1 {
		t.Fatalf("expected 1 trade, got %d", result.TotalTrades)
	}
	trade := result.Trades[0]
	if !trade.EntryTimestamp.Equal(signalTime.Add(5 * time.Minute)) {
		t.Errorf("expected entry at next trade bar, got %s", trade.EntryTimestamp)
	}
	if trade.EntryPrice != 250 {
		t.Errorf("expected entry at next trade open 250, got %f", trade.EntryPrice)
	}
}

func TestRunBacktest_TradeGapAtSignalTimestampSkipsEntry(t *testing.T) {
	signalCandles := makeMomentumSignalCandles()
	signalTime := signalCandles[len(signalCandles)-1].Timestamp
	tradeCandles := tradeCandlesAt(
		[]time.Time{signalTime.Add(5 * time.Minute), signalTime.Add(10 * time.Minute)},
		[]float64{250, 260},
	)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMomentum,
		LotSize:            1,
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, models.EngineInputs{
		Interval:      models.CandleInterval5M,
		SignalCandles: signalCandles,
		TradeCandles:  tradeCandles,
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades != 0 {
		t.Errorf("expected no entry when trade bar is missing at signal timestamp, got %d trades", result.TotalTrades)
	}
}

func TestRunBacktest_TradeSideExitWithoutSignalBar(t *testing.T) {
	signalCandles := makeMomentumSignalCandles()
	signalTime := signalCandles[len(signalCandles)-1].Timestamp
	tradeCandles := tradeCandlesAt(
		[]time.Time{signalTime, signalTime.Add(5 * time.Minute), signalTime.Add(10 * time.Minute)},
		[]float64{200, 250, 240},
	)
	tradeCandles[2].Low = 200
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMomentum,
		LotSize:            1,
		StopLossPct:        ptrFloat(5.0),
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, models.EngineInputs{
		Interval:      models.CandleInterval5M,
		SignalCandles: signalCandles,
		TradeCandles:  tradeCandles,
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades != 1 {
		t.Fatalf("expected 1 trade, got %d", result.TotalTrades)
	}
	trade := result.Trades[0]
	if trade.ExitReason != ExitReasonStopLoss {
		t.Fatalf("expected stop-loss exit on trade-side bar, got %s", trade.ExitReason)
	}
	if !trade.ExitTimestamp.Equal(signalTime.Add(10 * time.Minute)) {
		t.Errorf("expected exit on trade-side-only bar, got %s", trade.ExitTimestamp)
	}
}

func TestRunBacktest_SameInstrumentBaselineUsesNextBarOpen(t *testing.T) {
	candles := makeMomentumSignalCandles()
	signalTime := candles[len(candles)-1].Timestamp
	nextBar := tradeCandlesAt([]time.Time{signalTime.Add(5 * time.Minute)}, []float64{250})[0]
	candles = append(candles, nextBar)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMomentum,
		LotSize:            1,
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, sameStreamInputs(candles), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades != 1 {
		t.Fatalf("expected 1 trade, got %d", result.TotalTrades)
	}
	trade := result.Trades[0]
	if !trade.EntryTimestamp.Equal(signalTime.Add(5 * time.Minute)) {
		t.Errorf("expected same-stream baseline entry at next bar, got %s", trade.EntryTimestamp)
	}
	if trade.EntryPrice != 250 {
		t.Errorf("expected entry at next bar open 250, got %f", trade.EntryPrice)
	}
}

func TestRunBacktest_TradeSideTargetExitWithoutSignalBar(t *testing.T) {
	signalCandles := makeMomentumSignalCandles()
	signalTime := signalCandles[len(signalCandles)-1].Timestamp
	tradeCandles := tradeCandlesAt(
		[]time.Time{signalTime, signalTime.Add(5 * time.Minute), signalTime.Add(10 * time.Minute)},
		[]float64{200, 250, 251},
	)
	tradeCandles[2].High = 280
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMomentum,
		LotSize:            1,
		TargetPct:          ptrFloat(5.0),
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, models.EngineInputs{
		Interval:      models.CandleInterval5M,
		SignalCandles: signalCandles,
		TradeCandles:  tradeCandles,
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades != 1 {
		t.Fatalf("expected 1 trade, got %d", result.TotalTrades)
	}
	trade := result.Trades[0]
	if trade.ExitReason != ExitReasonTarget {
		t.Fatalf("expected target exit on trade-side bar, got %s", trade.ExitReason)
	}
	if !trade.ExitTimestamp.Equal(signalTime.Add(10 * time.Minute)) {
		t.Errorf("expected exit on trade-side-only bar, got %s", trade.ExitTimestamp)
	}
}

func TestRunBacktest_DailyIntervalDoesNotInferWeekendGap(t *testing.T) {
	base := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
	signalCandles := make([]models.Candle, 22)
	nextTradingDay := base
	for i := range signalCandles {
		signalCandles[i] = models.Candle{
			Timestamp: nextTradingDay,
			Open:      100,
			High:      101,
			Low:       99,
			Close:     100,
		}
		nextTradingDay = nextTradingDay.AddDate(0, 0, 1)
		for nextTradingDay.Weekday() == time.Saturday || nextTradingDay.Weekday() == time.Sunday {
			nextTradingDay = nextTradingDay.AddDate(0, 0, 1)
		}
	}
	signalCandles[21].Close = 110
	signalCandles[21].High = 111
	signalTime := signalCandles[21].Timestamp
	tradeCandles := tradeCandlesAt(
		[]time.Time{signalTime, signalTime.AddDate(0, 0, 1)},
		[]float64{200, 250},
	)
	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMomentum,
		LotSize:            1,
	}

	result, err := NewBacktester(zeroCharges{}).RunBacktest(s, models.EngineInputs{
		Interval:      models.CandleInterval1D,
		SignalCandles: signalCandles,
		TradeCandles:  tradeCandles,
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades != 1 {
		t.Fatalf("expected 1 trade, got %d", result.TotalTrades)
	}
	trade := result.Trades[0]
	if !trade.EntryTimestamp.Equal(signalTime.AddDate(0, 0, 1)) {
		t.Errorf("expected next-day entry using explicit 1d interval, got %s", trade.EntryTimestamp)
	}
}

// TestRunBacktest_AppliesCharges drives the engine with the real IndianRetailCharges
// calculator and asserts that charges are deducted from the per-trade and aggregate
// NetPnL. Uses a small OPTIDX BUY round-trip so STT/stamp leg mapping has to be right.
func TestRunBacktest_AppliesCharges(t *testing.T) {
	closes := make([]float64, 60)
	for i := 0; i < 25; i++ {
		closes[i] = 100
	}
	for i := 25; i < 60; i++ {
		closes[i] = 100 + float64(i-24)*1.5
	}
	candles := makeCandleSeries(closes, 5)

	s := &models.Strategy{
		EntryConditionType: models.EntryConditionMACrossover,
		InstrumentType:     models.InstrumentTypeOptionsIndex,
		LotSize:            50,
		NumberOfLots:       1,
	}

	result, err := NewBacktester(NewCharges()).RunBacktest(s, sameStreamInputs(candles), 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalTrades == 0 {
		t.Fatal("expected at least one trade")
	}
	if result.TotalCharges <= 0 {
		t.Fatalf("expected positive TotalCharges, got %.4f", result.TotalCharges)
	}
	if result.Slippage <= 0 {
		t.Fatalf("expected positive Slippage, got %.4f", result.Slippage)
	}
	if result.GrossPnL <= result.NetPnL {
		t.Fatalf("expected GrossPnL > NetPnL after costs; gross=%.4f net=%.4f charges=%.4f slippage=%.4f",
			result.GrossPnL, result.NetPnL, result.TotalCharges, result.Slippage)
	}
	if math.Abs((result.GrossPnL-result.TotalCharges-result.Slippage)-result.NetPnL) > 1 {
		t.Fatalf("invariant violated: gross−charges−slippage=%.4f, net=%.4f",
			result.GrossPnL-result.TotalCharges-result.Slippage, result.NetPnL)
	}

	tradeNonSlippageCharges := 0.0
	tradeSlippage := 0.0
	for _, tr := range result.Trades {
		if tr.TotalCharges <= 0 {
			t.Errorf("trade has non-positive TotalCharges: %+v", tr)
		}
		sum := tr.Slippage + tr.Brokerage + tr.STT + tr.ExchangeFees + tr.SEBIFees + tr.GST + tr.StampDuty
		if math.Abs(sum-tr.TotalCharges) > tolerance {
			t.Errorf("trade charges sum mismatch: components=%.6f totalCharges=%.6f", sum, tr.TotalCharges)
		}
		if math.Abs((tr.GrossPnL-tr.TotalCharges)-tr.NetPnL) > tolerance {
			t.Errorf("trade pnl invariant violated: gross=%.4f charges=%.4f net=%.4f",
				tr.GrossPnL, tr.TotalCharges, tr.NetPnL)
		}
		tradeNonSlippageCharges += tr.TotalCharges - tr.Slippage
		tradeSlippage += tr.Slippage
	}
	if math.Abs(tradeNonSlippageCharges-result.TotalCharges) > tolerance {
		t.Errorf("aggregate TotalCharges should exclude slippage: trade non-slippage sum=%.6f result=%.6f",
			tradeNonSlippageCharges, result.TotalCharges)
	}
	if math.Abs(tradeSlippage-result.Slippage) > tolerance {
		t.Errorf("aggregate Slippage mismatch: trade slippage sum=%.6f result=%.6f",
			tradeSlippage, result.Slippage)
	}
}
