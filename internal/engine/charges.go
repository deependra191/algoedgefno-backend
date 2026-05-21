// Package engine — charges.go contains the Indian retail F&O cost model used to
// deduct brokerage and statutory charges from every backtest trade.
//
// Rates as of 2026-05-11, verified against Zerodha's published charges schedule.
// F&O STT reflects the Finance Act 2026 hike effective 2026-04-01
// (futures 0.02% → 0.05% sell-side; options 0.10% → 0.15% sell-side on premium).
// NSE transaction charges: equity 0.00307%, futures 0.00183%, options 0.03553% on premium.
// Historical charge modelling is intentionally out of scope for B14 — today's
// constants are applied to every trade date regardless of when it occurred.
//
// Brokerage uses the project's generic discount-broker model:
//
//	min(₹20, 0.03% × turnover) per leg.
//
// Zerodha actually charges flat ₹20 per options leg with no percentage step,
// so this model under-charges brokerage on tiny option turnovers (< ₹66,667 per leg)
// by up to ₹20 vs Zerodha's actual schedule. Acceptable simplification for B14.
//
// Slippage modelling — Stage 1 (current): slippage rates are 0, so backtests
// report Slippage = 0 and NetPnL = GrossPnL − TotalCharges. The previous
// %-of-turnover slippage model was structurally wrong — real fill slippage
// scales with tick size × lot, not with notional, so a single per-segment %
// systematically overstated slippage on index futures (~70× observed) while
// understating it on low-price / high-lot stock futures by a similar factor.
// Rather than ship a calibrated-but-still-wrong default, we zero it out and
// will reintroduce slippage as a user-provided input (Stage 2, Android UI)
// and eventually a provider-aware data-driven model (Stage 3, post vendor
// migration). The slippage line is preserved in the formula so reintroducing
// real rates is a one-line change.
//
// UI obligation: backtest result screens must surface a disclaimer that
// slippage is not modelled in Stage 1 and actual fills may differ from
// reported NetPnL.
package engine

import (
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// Slippage percentages per side (one-way), keyed on Strategy.InstrumentType.
// A round-trip pays slipPct × (entryPrice + exitPrice) × qty.
//
// Stage 1: all rates are 0. The previous %-of-turnover model was structurally
// wrong across the instrument range (see the file-level doc header). Stage 2
// will replace these with a user-provided per-run input; Stage 3 will derive
// them from instrument-master + market-data metadata. The constants remain so
// the slippage line in Compute() continues to exercise the same code path.
//
// TODO(stage-2): wire a user-provided slippage value through the backtest
// request DTO and Android UI; expose disclaimer copy on the result screen.
// TODO(stage-3): replace with a provider-aware model (tick size, spread,
// liquidity, time-of-day) once richer instrument-master data lands.
const (
	slipPctEQ     = 0.0
	slipPctFUTIDX = 0.0
	slipPctFUTSTK = 0.0
	slipPctOPTIDX = 0.0
	slipPctOPTSTK = 0.0
)

// Securities Transaction Tax — applies to the SELL leg only.
const (
	sttPctEQ  = 0.00025 // 0.025% on equity intraday sell turnover
	sttPctFUT = 0.0005  // 0.05% on futures sell turnover (post FA 2026)
	sttPctOPT = 0.0015  // 0.15% on options sell premium turnover (post FA 2026)
)

// NSE transaction charges — applied on both legs.
const (
	exchPctEQ  = 0.0000307 // 0.00307%
	exchPctFUT = 0.0000183 // 0.00183%
	exchPctOPT = 0.0003553 // 0.03553% on premium
)

// SEBI turnover fee — applied on both legs, flat across segments.
const sebiPct = 0.000001 // 0.0001% (₹10 per ₹1 crore turnover)

// GST rate — applied on (brokerage + exchange + SEBI) only.
// GST explicitly excludes STT and stamp duty.
const gstPct = 0.18

// Stamp duty — applied on the BUY leg only.
const (
	stampPctEQ  = 0.00003 // 0.003%
	stampPctFUT = 0.00002 // 0.002%
	stampPctOPT = 0.00003 // 0.003%
)

// Brokerage — discount-broker model: min(₹20, 0.03% × turnover) per leg.
const (
	brokerageFlatPerLeg = 20.0
	brokeragePct        = 0.0003
)

var _ models.ChargeCalculator = IndianRetailCharges{}

// IndianRetailCharges implements the Indian retail F&O cost model documented at
// the top of this file. It is stateless and safe for concurrent use.
type IndianRetailCharges struct{}

// NewCharges returns a fresh IndianRetailCharges. Always non-nil.
func NewCharges() *IndianRetailCharges {
	return &IndianRetailCharges{}
}

// Compute implements models.ChargeCalculator. See the interface for the contract.
// Returns a zero ChargeBreakdown when segment is not a recognised InstrumentType
// so a misconfigured strategy fails open with no charges rather than panicking.
func (IndianRetailCharges) Compute(segment string, side models.OrderSide, entryPrice, exitPrice float64, quantity int) models.ChargeBreakdown {
	slipPct, exchPct, sttPct, stampPct, ok := rateTable(segment)
	if !ok {
		return models.ChargeBreakdown{}
	}

	qty := float64(quantity)
	entryTurnover := entryPrice * qty
	exitTurnover := exitPrice * qty
	totalTurnover := entryTurnover + exitTurnover

	slippage := slipPct * (entryPrice + exitPrice) * qty
	brokerage := perLegBrokerage(entryTurnover) + perLegBrokerage(exitTurnover)
	exchange := exchPct * totalTurnover
	sebi := sebiPct * totalTurnover

	// STT applies on the SELL leg; stamp duty applies on the BUY leg.
	// For a long, BUY is entry / SELL is exit; for a short, the mapping mirrors.
	var sttTurnover, stampTurnover float64
	if side == models.OrderSideBuy {
		sttTurnover = exitTurnover
		stampTurnover = entryTurnover
	} else {
		sttTurnover = entryTurnover
		stampTurnover = exitTurnover
	}
	stt := sttPct * sttTurnover
	stamp := stampPct * stampTurnover

	gst := gstPct * (brokerage + exchange + sebi)
	total := slippage + brokerage + stt + exchange + sebi + gst + stamp

	return models.ChargeBreakdown{
		Slippage:     slippage,
		Brokerage:    brokerage,
		STT:          stt,
		ExchangeFees: exchange,
		SEBIFees:     sebi,
		GST:          gst,
		StampDuty:    stamp,
		Total:        total,
	}
}

func perLegBrokerage(turnover float64) float64 {
	pct := brokeragePct * turnover
	if pct < brokerageFlatPerLeg {
		return pct
	}
	return brokerageFlatPerLeg
}

// rateTable resolves the (slippage, exchange, STT, stamp) rates for a segment.
// ok is false when the segment is unknown — callers MUST return a zero breakdown.
func rateTable(segment string) (slipPct, exchPct, sttPct, stampPct float64, ok bool) {
	switch segment {
	case models.InstrumentTypeEquity:
		return slipPctEQ, exchPctEQ, sttPctEQ, stampPctEQ, true
	case models.InstrumentTypeFuturesIndex, models.InstrumentTypeFuturesIndexCont:
		return slipPctFUTIDX, exchPctFUT, sttPctFUT, stampPctFUT, true
	case models.InstrumentTypeFuturesStock, models.InstrumentTypeFuturesStockCont:
		return slipPctFUTSTK, exchPctFUT, sttPctFUT, stampPctFUT, true
	case models.InstrumentTypeOptionsIndex:
		return slipPctOPTIDX, exchPctOPT, sttPctOPT, stampPctOPT, true
	case models.InstrumentTypeOptionsStock:
		return slipPctOPTSTK, exchPctOPT, sttPctOPT, stampPctOPT, true
	default:
		return 0, 0, 0, 0, false
	}
}
