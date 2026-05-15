package models

import (
	"encoding/json"
	"testing"
)

// TestTradeUnmarshalJSON_LegacyPnL asserts the back-compat shim for pre-B14
// trades_json blobs: a legacy "PnL" key mirrors to both NetPnL and GrossPnL
// and the charge breakdown stays at zero.
func TestTradeUnmarshalJSON_LegacyPnL(t *testing.T) {
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

	var tr Trade
	if err := json.Unmarshal(raw, &tr); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tr.NetPnL != 123.45 {
		t.Errorf("NetPnL: got %v, want 123.45", tr.NetPnL)
	}
	if tr.GrossPnL != 123.45 {
		t.Errorf("GrossPnL: got %v, want 123.45", tr.GrossPnL)
	}
	if tr.TotalCharges != 0 {
		t.Errorf("TotalCharges: got %v, want 0", tr.TotalCharges)
	}
	if tr.Quantity != 50 || tr.EntryPrice != 100 || tr.ExitPrice != 105 {
		t.Errorf("scalar fields not preserved: %+v", tr)
	}
}

// TestTradeUnmarshalJSON_NewFormat asserts a post-B14 JSON object (no "PnL"
// key, explicit NetPnL/GrossPnL/TotalCharges) round-trips without the legacy
// shim clobbering anything — including the case where NetPnL is legitimately 0.
func TestTradeUnmarshalJSON_NewFormat(t *testing.T) {
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
		"TotalCharges":   72,
		"NetPnL":         928,
		"Reason":         "MA bullish",
		"ExitReason":     "target"
	}`)

	var tr Trade
	if err := json.Unmarshal(raw, &tr); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tr.GrossPnL != 1000 || tr.NetPnL != 928 || tr.TotalCharges != 72 {
		t.Errorf("new-format fields not preserved: gross=%v net=%v charges=%v",
			tr.GrossPnL, tr.NetPnL, tr.TotalCharges)
	}
	if tr.Slippage != 11 || tr.Brokerage != 40 || tr.STT != 9 {
		t.Errorf("charge breakdown not preserved: %+v", tr)
	}
}

// TestTradeUnmarshalJSON_NewFormat_ZeroNetPnL guards against a future regression
// where the shim might fire on a real B14 break-even trade just because NetPnL
// happens to be 0. The shim must only fire when the legacy "PnL" key is present.
func TestTradeUnmarshalJSON_NewFormat_ZeroNetPnL(t *testing.T) {
	raw := []byte(`{
		"Side":         "BUY",
		"GrossPnL":     50,
		"TotalCharges": 50,
		"NetPnL":       0
	}`)

	var tr Trade
	if err := json.Unmarshal(raw, &tr); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if tr.GrossPnL != 50 {
		t.Errorf("GrossPnL: got %v, want 50", tr.GrossPnL)
	}
	if tr.NetPnL != 0 {
		t.Errorf("NetPnL: got %v, want 0", tr.NetPnL)
	}
	if tr.TotalCharges != 50 {
		t.Errorf("TotalCharges: got %v, want 50", tr.TotalCharges)
	}
}
