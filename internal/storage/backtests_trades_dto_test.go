package storage

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// TestTradePersistUnmarshalLegacyPnL asserts the back-compat shim for pre-B14
// trades_json blobs: a legacy "PnL" key mirrors to both NetPnL and GrossPnL
// and the charge breakdown stays at zero.
func TestTradePersistUnmarshalLegacyPnL(t *testing.T) {
	raw := []byte(`{
		"EntryTimestamp": "2025-01-02T09:15:00Z",
		"ExitTimestamp":  "2025-01-02T09:30:00Z",
		"Side":           "BUY",
		"Quantity":       50,
		"EntryPrice":     100,
		"ExitPrice":      105,
		"PnL":            123.45,
		"Reason":         "MA bullish",
		"ExitReason":     "target"
	}`)

	var tp tradePersist
	if err := json.Unmarshal(raw, &tp); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tp.NetPnL != 123.45 {
		t.Errorf("NetPnL: got %v, want 123.45", tp.NetPnL)
	}
	if tp.GrossPnL != 123.45 {
		t.Errorf("GrossPnL: got %v, want 123.45", tp.GrossPnL)
	}
	if tp.TotalCharges != 0 {
		t.Errorf("TotalCharges: got %v, want 0", tp.TotalCharges)
	}
	if tp.Quantity != 50 || tp.EntryPrice != 100 || tp.ExitPrice != 105 {
		t.Errorf("scalar fields not preserved: %+v", tp)
	}
}

// TestTradePersistUnmarshalNewFormat asserts a post-B14 JSON object (no "PnL"
// key, explicit NetPnL/GrossPnL/TotalCharges) round-trips without the legacy
// shim clobbering anything.
func TestTradePersistUnmarshalNewFormat(t *testing.T) {
	raw := []byte(`{
		"EntryTimestamp": "2025-01-02T09:15:00Z",
		"ExitTimestamp":  "2025-01-02T09:30:00Z",
		"Side":           "BUY",
		"Quantity":       50,
		"EntryPrice":     100,
		"ExitPrice":      120,
		"GrossPnL":       1000,
		"Slippage":       11,
		"Brokerage":      40,
		"STT":            9,
		"ExchangeFees":   3.91,
		"SEBIFees":       0.011,
		"GST":            7.91,
		"StampDuty":      0.15,
		"TotalCharges":   61,
		"NetPnL":         928,
		"Reason":         "MA bullish",
		"ExitReason":     "target"
	}`)

	var tp tradePersist
	if err := json.Unmarshal(raw, &tp); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tp.GrossPnL != 1000 || tp.NetPnL != 928 || tp.TotalCharges != 61 {
		t.Errorf("new-format fields not preserved: gross=%v net=%v charges=%v",
			tp.GrossPnL, tp.NetPnL, tp.TotalCharges)
	}
	if tp.Slippage != 11 || tp.Brokerage != 40 || tp.STT != 9 {
		t.Errorf("charge breakdown not preserved: %+v", tp)
	}
}

// TestTradePersistUnmarshalNewFormatZeroNetPnL guards against a future regression
// where the shim might fire on a real B14 break-even trade just because NetPnL
// happens to be 0. The shim must only fire when the legacy "PnL" key is present.
func TestTradePersistUnmarshalNewFormatZeroNetPnL(t *testing.T) {
	raw := []byte(`{
		"Side":         "BUY",
		"GrossPnL":     50,
		"TotalCharges": 50,
		"NetPnL":       0
	}`)

	var tp tradePersist
	if err := json.Unmarshal(raw, &tp); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tp.GrossPnL != 50 {
		t.Errorf("GrossPnL: got %v, want 50", tp.GrossPnL)
	}
	if tp.NetPnL != 0 {
		t.Errorf("NetPnL: got %v, want 0", tp.NetPnL)
	}
	if tp.TotalCharges != 50 {
		t.Errorf("TotalCharges: got %v, want 50", tp.TotalCharges)
	}
}

// TestDecodeTradesJSONLegacyArray exercises the full decodeTradesJSON storage path
// with a legacy trades_json array (pre-B14 "PnL" key). This locks the real DB read
// path end-to-end: the legacy shim in tradePersist.UnmarshalJSON must fire, mirror
// "PnL" to both NetPnL and GrossPnL, and leave TotalCharges at zero.
func TestDecodeTradesJSONLegacyArray(t *testing.T) {
	raw := []byte(`[
		{
			"PnL": -568.9,
			"Side": "BUY",
			"Reason": "Close crossed above VWAP",
			"Quantity": 2,
			"ExitPrice": 24718.6,
			"EntryPrice": 25003.05,
			"ExitReason": "signal_reversal",
			"ExitTimestamp": "2025-06-13T05:30:00+05:30",
			"EntryTimestamp": "2025-06-06T05:30:00+05:30"
		}
	]`)

	trades, err := decodeTradesJSON(raw)
	if err != nil {
		t.Fatalf("decodeTradesJSON: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}
	tr := trades[0]
	if tr.NetPnL != -568.9 {
		t.Errorf("NetPnL: got %v, want -568.9", tr.NetPnL)
	}
	if tr.GrossPnL != -568.9 {
		t.Errorf("GrossPnL: got %v, want -568.9", tr.GrossPnL)
	}
	if tr.TotalCharges != 0 {
		t.Errorf("TotalCharges: got %v, want 0", tr.TotalCharges)
	}
	if tr.Quantity != 2 {
		t.Errorf("Quantity: got %v, want 2", tr.Quantity)
	}
}

// TestTradePersistUnmarshalLegacyFixture verifies the exact pre-B14 JSON shape
// from a real stored row (backtest_runs.id = 2489af42-abea-4efb-bfa6-c80b7f468a08).
func TestTradePersistUnmarshalLegacyFixture(t *testing.T) {
	raw := []byte(`{
		"PnL": -568.9,
		"Side": "BUY",
		"Reason": "Close crossed above VWAP",
		"Quantity": 2,
		"ExitPrice": 24718.6,
		"EntryPrice": 25003.05,
		"ExitReason": "signal_reversal",
		"ExitTimestamp": "2025-06-13T05:30:00+05:30",
		"EntryTimestamp": "2025-06-06T05:30:00+05:30"
	}`)

	var tp tradePersist
	if err := json.Unmarshal(raw, &tp); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tp.NetPnL != -568.9 {
		t.Errorf("NetPnL: got %v, want -568.9", tp.NetPnL)
	}
	if tp.GrossPnL != -568.9 {
		t.Errorf("GrossPnL: got %v, want -568.9", tp.GrossPnL)
	}
	if tp.TotalCharges != 0 {
		t.Errorf("TotalCharges: got %v, want 0", tp.TotalCharges)
	}
}

// TestEncodeTradesJSONKeyShape asserts that encodeTradesJSON produces PascalCase JSON
// keys matching the on-disk contract. A field-tag rename would silently break Android
// clients reading legacy rows; this test catches it before it reaches storage.
func TestEncodeTradesJSONKeyShape(t *testing.T) {
	trades := []models.Trade{{
		Side:         models.OrderSideBuy,
		GrossPnL:     1000,
		NetPnL:       928,
		TotalCharges: 61,
		Slippage:     11,
		Brokerage:    40,
		STT:          9,
		ExchangeFees: 3.91,
		SEBIFees:     0.011,
		GST:          7.91,
		StampDuty:    0.15,
		Reason:       "test",
		ExitReason:   "target",
	}}

	b, err := encodeTradesJSON(trades)
	if err != nil {
		t.Fatalf("encodeTradesJSON: %v", err)
	}
	for _, key := range []string{
		`"GrossPnL"`, `"NetPnL"`, `"TotalCharges"`,
		`"Slippage"`, `"Brokerage"`, `"STT"`,
		`"ExchangeFees"`, `"SEBIFees"`, `"GST"`, `"StampDuty"`,
		`"EntryTimestamp"`, `"ExitTimestamp"`, `"Side"`, `"Quantity"`,
		`"EntryPrice"`, `"ExitPrice"`, `"Reason"`, `"ExitReason"`,
	} {
		if !bytes.Contains(b, []byte(key)) {
			t.Errorf("encoded JSON missing expected key %s", key)
		}
	}
}

// TestTradePersistRoundTrip verifies that models.Trade → tradePersist → JSON →
// tradePersist → models.Trade preserves every field within float-rounding tolerance.
func TestTradePersistRoundTrip(t *testing.T) {
	entry := time.Date(2025, 3, 10, 9, 15, 0, 0, time.UTC)
	exit := time.Date(2025, 3, 10, 9, 30, 0, 0, time.UTC)

	original := models.Trade{
		EntryTimestamp: entry,
		ExitTimestamp:  exit,
		Side:           models.OrderSideBuy,
		Quantity:       75,
		EntryPrice:     22500.5,
		ExitPrice:      22650.25,
		GrossPnL:       11231.25,
		Slippage:       45.0,
		Brokerage:      80.0,
		STT:            22.5,
		ExchangeFees:   9.5,
		SEBIFees:       0.05,
		GST:            16.2,
		StampDuty:      0.3,
		TotalCharges:   173.55,
		NetPnL:         11012.70,
		Reason:         "MA crossover",
		ExitReason:     "target",
	}

	encoded, err := encodeTradesJSON([]models.Trade{original})
	if err != nil {
		t.Fatalf("encodeTradesJSON: %v", err)
	}

	decoded, err := decodeTradesJSON(encoded)
	if err != nil {
		t.Fatalf("decodeTradesJSON: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(decoded))
	}

	got := decoded[0]
	if !got.EntryTimestamp.Equal(original.EntryTimestamp) {
		t.Errorf("EntryTimestamp: got %v, want %v", got.EntryTimestamp, original.EntryTimestamp)
	}
	if !got.ExitTimestamp.Equal(original.ExitTimestamp) {
		t.Errorf("ExitTimestamp: got %v, want %v", got.ExitTimestamp, original.ExitTimestamp)
	}
	if got.Side != original.Side {
		t.Errorf("Side: got %v, want %v", got.Side, original.Side)
	}
	if got.Quantity != original.Quantity {
		t.Errorf("Quantity: got %v, want %v", got.Quantity, original.Quantity)
	}
	if got.EntryPrice != original.EntryPrice {
		t.Errorf("EntryPrice: got %v, want %v", got.EntryPrice, original.EntryPrice)
	}
	if got.ExitPrice != original.ExitPrice {
		t.Errorf("ExitPrice: got %v, want %v", got.ExitPrice, original.ExitPrice)
	}
	if got.GrossPnL != original.GrossPnL {
		t.Errorf("GrossPnL: got %v, want %v", got.GrossPnL, original.GrossPnL)
	}
	if got.NetPnL != original.NetPnL {
		t.Errorf("NetPnL: got %v, want %v", got.NetPnL, original.NetPnL)
	}
	if got.TotalCharges != original.TotalCharges {
		t.Errorf("TotalCharges: got %v, want %v", got.TotalCharges, original.TotalCharges)
	}
	if got.Slippage != original.Slippage {
		t.Errorf("Slippage: got %v, want %v", got.Slippage, original.Slippage)
	}
	if got.Brokerage != original.Brokerage {
		t.Errorf("Brokerage: got %v, want %v", got.Brokerage, original.Brokerage)
	}
	if got.Reason != original.Reason {
		t.Errorf("Reason: got %v, want %v", got.Reason, original.Reason)
	}
	if got.ExitReason != original.ExitReason {
		t.Errorf("ExitReason: got %v, want %v", got.ExitReason, original.ExitReason)
	}
}
