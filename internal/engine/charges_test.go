package engine

import (
	"math"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const chargeTolerance = 1e-6

func assertInDelta(t *testing.T, name string, got, want, delta float64) {
	t.Helper()
	if math.Abs(got-want) > delta {
		t.Errorf("%s: got %.8f, want %.8f (Δ=%.8f, tol=%.8f)", name, got, want, math.Abs(got-want), delta)
	}
}

// OPTIDX long: BUY @ 100, SELL @ 120, qty 50.
// STT must sit on the exit (SELL) leg; stamp must sit on the entry (BUY) leg.
func TestCharges_OPTIDX_Long_LegMapping(t *testing.T) {
	cb := NewCharges().Compute(models.InstrumentTypeOptionsIndex, models.OrderSideBuy, 100, 120, 50)

	assertInDelta(t, "STT", cb.STT, 0.0015*120*50, chargeTolerance)              // 9.0
	assertInDelta(t, "StampDuty", cb.StampDuty, 0.00003*100*50, chargeTolerance) // 0.15
	assertInDelta(t, "ExchangeFees", cb.ExchangeFees, 0.0003553*(100*50+120*50), chargeTolerance)
	assertInDelta(t, "SEBIFees", cb.SEBIFees, 0.000001*(100*50+120*50), chargeTolerance)

	// Sanity: stamp NOT applied on exit.
	if cb.StampDuty > 0.00003*120*50 {
		t.Errorf("StampDuty=%.6f looks like it was applied to exit turnover", cb.StampDuty)
	}
}

// FUTIDX short: SELL @ 20000, BUY @ 19900, qty 25.
// STT must sit on the entry (SELL) leg; stamp must sit on the exit (BUY) leg.
func TestCharges_FUTIDX_Short_LegMapping(t *testing.T) {
	cb := NewCharges().Compute(models.InstrumentTypeFuturesIndex, models.OrderSideSell, 20000, 19900, 25)

	assertInDelta(t, "STT", cb.STT, 0.0005*20000*25, chargeTolerance)              // 250.0 (entry SELL)
	assertInDelta(t, "StampDuty", cb.StampDuty, 0.00002*19900*25, chargeTolerance) // 9.95 (exit BUY)

	// STT NOT on exit, stamp NOT on entry.
	if math.Abs(cb.STT-0.0005*19900*25) < chargeTolerance {
		t.Error("STT was applied to BUY leg instead of SELL leg")
	}
	if math.Abs(cb.StampDuty-0.00002*20000*25) < chargeTolerance {
		t.Error("StampDuty was applied to SELL leg instead of BUY leg")
	}
}

// OPTIDX short: SELL @ 120, BUY @ 100, qty 50 — leg mapping mirror of the long case.
// Highest-risk site for an asymmetry bug.
func TestCharges_OPTIDX_Short_LegMapping(t *testing.T) {
	cb := NewCharges().Compute(models.InstrumentTypeOptionsIndex, models.OrderSideSell, 120, 100, 50)

	assertInDelta(t, "STT", cb.STT, 0.0015*120*50, chargeTolerance)              // 9.0 on entry SELL
	assertInDelta(t, "StampDuty", cb.StampDuty, 0.00003*100*50, chargeTolerance) // 0.15 on exit BUY

	if math.Abs(cb.STT-0.0015*100*50) < chargeTolerance {
		t.Error("STT was applied to BUY leg (exit) instead of SELL leg (entry)")
	}
	if math.Abs(cb.StampDuty-0.00003*120*50) < chargeTolerance {
		t.Error("StampDuty was applied to SELL leg (entry) instead of BUY leg (exit)")
	}
}

// Brokerage saturates at ₹20 per leg once turnover ≥ ₹66,667.
func TestCharges_BrokerageFlatCap(t *testing.T) {
	// Equity at ₹1000 × 100 qty = ₹100,000 per leg, both legs above cap.
	cb := NewCharges().Compute(models.InstrumentTypeEquity, models.OrderSideBuy, 1000, 1010, 100)
	assertInDelta(t, "Brokerage (capped)", cb.Brokerage, 40.0, chargeTolerance)
}

// Brokerage falls below the cap on tiny turnovers — proves the min() is doing work.
func TestCharges_BrokeragePercentBelowCap(t *testing.T) {
	// 1 share at ₹100 → turnover = ₹100, 0.0003 × 100 = ₹0.03 per leg, total ₹0.06.
	cb := NewCharges().Compute(models.InstrumentTypeEquity, models.OrderSideBuy, 100, 110, 1)
	if cb.Brokerage >= 40.0 {
		t.Fatalf("expected brokerage well below cap, got %.4f", cb.Brokerage)
	}
	wantPerLeg := 0.0003*100 + 0.0003*110
	assertInDelta(t, "Brokerage (percent)", cb.Brokerage, wantPerLeg, chargeTolerance)
}

// GST base must exclude STT and stamp duty; it equals 18% × (brokerage + exchange + SEBI).
func TestCharges_GSTBaseExcludesSTTAndStamp(t *testing.T) {
	cb := NewCharges().Compute(models.InstrumentTypeOptionsIndex, models.OrderSideBuy, 200, 220, 75)

	if cb.STT == 0 || cb.StampDuty == 0 {
		t.Fatalf("test fixture should produce non-zero STT and StampDuty; got STT=%.6f stamp=%.6f", cb.STT, cb.StampDuty)
	}
	wantBase := cb.Brokerage + cb.ExchangeFees + cb.SEBIFees
	assertInDelta(t, "GST base", cb.GST/gstPct, wantBase, chargeTolerance)

	// Sanity guard: a buggy implementation that included STT or stamp would give a strictly larger base.
	if math.Abs(cb.GST/gstPct-(wantBase+cb.STT+cb.StampDuty)) < chargeTolerance {
		t.Error("GST base appears to include STT and/or stamp")
	}
}

// Unknown segment returns the zero ChargeBreakdown.
func TestCharges_UnknownSegment(t *testing.T) {
	cb := NewCharges().Compute("MUTUAL_FUND", models.OrderSideBuy, 100, 110, 10)
	if cb != (models.ChargeBreakdown{}) {
		t.Fatalf("expected zero breakdown for unknown segment, got %+v", cb)
	}
}

// Total invariant: every individual component sums to Total within float-rounding tolerance.
func TestCharges_TotalInvariant(t *testing.T) {
	cases := []struct {
		name    string
		segment string
		side    models.OrderSide
		entry   float64
		exit    float64
		qty     int
	}{
		{"EQ long", models.InstrumentTypeEquity, models.OrderSideBuy, 1500, 1520, 50},
		{"FUTSTK short", models.InstrumentTypeFuturesStock, models.OrderSideSell, 2500, 2480, 600},
		{"OPTIDX long", models.InstrumentTypeOptionsIndex, models.OrderSideBuy, 180, 250, 75},
		{"OPTSTK short", models.InstrumentTypeOptionsStock, models.OrderSideSell, 300, 280, 250},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cb := NewCharges().Compute(tc.segment, tc.side, tc.entry, tc.exit, tc.qty)
			sum := cb.Brokerage + cb.STT + cb.ExchangeFees + cb.SEBIFees + cb.GST + cb.StampDuty
			assertInDelta(t, "Total invariant", cb.Total, sum, chargeTolerance)
		})
	}
}
