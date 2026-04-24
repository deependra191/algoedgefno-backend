package handlers

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func TestBacktestResultResponseJSONShape(t *testing.T) {
	resp := backtestResultResponse{
		ID:         "abc",
		ReturnPct:  11.2,
		WinRate:    49,
		TradeCount: 22,
		Trades:     []backtestTradeResponse{},
	}
	keys := jsonKeys(t, resp)
	want := []string{"id", "returnPct", "tradeCount", "trades", "winRate"}
	assertKeysEqual(t, keys, want)
}

func TestBacktestTradeResponseJSONShape(t *testing.T) {
	resp := backtestTradeResponse{
		EntryTs:    "2025-01-02T09:15:00Z",
		ExitTs:     "2025-01-02T09:30:00Z",
		Side:       "BUY",
		Quantity:   50,
		EntryPrice: 100.0,
		ExitPrice:  105.0,
		Pnl:        250.0,
		Reason:     "MA crossover bullish",
		ExitReason: "target",
	}
	keys := jsonKeys(t, resp)
	want := []string{"entryPrice", "entryTs", "exitPrice", "exitReason", "exitTs", "pnl", "quantity", "reason", "side"}
	assertKeysEqual(t, keys, want)
}

func TestToBacktestResultResponse(t *testing.T) {
	id := uuid.New()
	capital := 200000.0
	pnl := 22400.0
	total := 22
	wins := 11
	losses := 11
	dd := 0.05

	trades := []models.Trade{
		{
			EntryTimestamp: time.Date(2025, 1, 2, 9, 15, 0, 0, time.UTC),
			ExitTimestamp:  time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC),
			Side:           models.OrderSideBuy,
			Quantity:       50,
			EntryPrice:     100.0,
			ExitPrice:      105.0,
			PnL:            250.0,
			Reason:         "MA crossover bullish",
			ExitReason:     "target",
		},
	}
	tradesJSON, _ := json.Marshal(trades)

	run := &models.BacktestRun{
		ID:          id,
		NetPnl:      &pnl,
		TotalTrades: &total,
		WinCount:    &wins,
		LossCount:   &losses,
		MaxDrawdown: &dd,
		Capital:     &capital,
		Trades:      tradesJSON,
	}

	resp := toBacktestResultResponse(run)
	if resp.ID != id.String() {
		t.Errorf("expected ID %s, got %s", id, resp.ID)
	}
	if math.Abs(resp.ReturnPct-11.2) > floatTolerance {
		t.Errorf("expected returnPct 11.2, got %f", resp.ReturnPct)
	}
	if resp.WinRate != 50 {
		t.Errorf("expected winRate 50, got %d", resp.WinRate)
	}
	if resp.TradeCount != 22 {
		t.Errorf("expected tradeCount 22, got %d", resp.TradeCount)
	}
	if len(resp.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(resp.Trades))
	}
	tr := resp.Trades[0]
	if tr.Side != "BUY" {
		t.Errorf("expected side BUY, got %s", tr.Side)
	}
	if tr.EntryPrice != 100.0 {
		t.Errorf("expected entryPrice 100, got %f", tr.EntryPrice)
	}
	if tr.Pnl != 250.0 {
		t.Errorf("expected pnl 250, got %f", tr.Pnl)
	}
}

func TestToBacktestTradeResponses_EmptyJSON(t *testing.T) {
	trades := toBacktestTradeResponses(nil)
	if len(trades) != 0 {
		t.Errorf("expected empty trades, got %d", len(trades))
	}
}

func TestToBacktestTradeResponses_InvalidJSON(t *testing.T) {
	trades := toBacktestTradeResponses(json.RawMessage(`invalid`))
	if len(trades) != 0 {
		t.Errorf("expected empty trades on invalid JSON, got %d", len(trades))
	}
}

func TestBacktestSubmitRequestJSONShape(t *testing.T) {
	resp := backtestSubmitRequest{
		StrategyID: "ma_crossover",
		Underlying: "NIFTY",
		From:       "2025-01-01",
		To:         "2025-06-30",
		Lots:       2,
		Capital:    200000,
	}
	keys := jsonKeys(t, resp)
	want := []string{"capital", "from", "lots", "strategyId", "to", "underlying"}
	assertKeysEqual(t, keys, want)
}
