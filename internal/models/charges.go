package models

// ChargeBreakdown is the per–round-trip cost stack deducted from a backtest trade.
// All values are absolute rupee amounts (always non-negative).
// Total is the sum of every other field — implementations MUST maintain this invariant.
// NetPnL is computed by the engine as GrossPnL − Total; Total itself never includes PnL.
type ChargeBreakdown struct {
	Slippage     float64
	Brokerage    float64
	STT          float64
	ExchangeFees float64
	SEBIFees     float64
	GST          float64
	StampDuty    float64
	Total        float64
}

// ChargeCalculator computes the cost stack for a single completed round-trip trade.
//
// segment selects the rate table. It accepts any InstrumentType value (EQ, FUTIDX,
// FUTIDX_CONT, FUTSTK, FUTSTK_CONT, OPTIDX, OPTSTK); unknown values MUST return a
// zero ChargeBreakdown rather than panic, so a misconfigured strategy fails open
// rather than corrupting P&L.
//
// side is the direction of the entry leg. The implementation is responsible for
// applying SELL-only charges (STT) and BUY-only charges (stamp duty) to the correct
// leg — for a long (BUY entry, SELL exit) STT applies on exit and stamp on entry;
// for a short (SELL entry, BUY exit) the mapping mirrors.
//
// entryPrice and exitPrice are the executed prices for one unit of the underlying
// (for options, the premium); quantity is the round-trip size in units (lotSize × lots).
// Implementations MUST return a breakdown where Total equals the sum of every other
// component within float-rounding tolerance.
type ChargeCalculator interface {
	Compute(segment string, side OrderSide, entryPrice, exitPrice float64, quantity int) ChargeBreakdown
}
