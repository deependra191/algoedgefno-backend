package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OrderSide represents the direction of a trade entry.
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// Trade is a single completed round-trip entry+exit produced by the backtest engine.
// GrossPnL is the frictionless price-move profit; NetPnL is GrossPnL − TotalCharges and
// is the value Android renders as "pnl". The individual charge fields are populated by
// the engine via the ChargeCalculator so the UI can show the full cost stack per trade.
type Trade struct {
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	Side           OrderSide
	Quantity       int
	EntryPrice     float64
	ExitPrice      float64
	GrossPnL       float64
	Slippage       float64
	Brokerage      float64
	STT            float64
	ExchangeFees   float64
	SEBIFees       float64
	GST            float64
	StampDuty      float64
	TotalCharges   float64
	NetPnL         float64
	Reason         string
	ExitReason     string
}

// TradeStats holds derived performance metrics computed once from a completed trade list.
// Pointer fields are nil when mathematically undefined (e.g. no losing trades → ProfitFactor is nil).
type TradeStats struct {
	AvgWin            *float64
	AvgLoss           *float64
	BestTrade         *float64
	WorstTrade        *float64
	AvgPnlPerTrade    *float64
	AvgHoldingMinutes *float64
	ProfitFactor      *float64
	RewardRisk        *float64
	LongestWinStreak  int
	LongestLossStreak int
	TradesPerWeek     float64
}

// ChartPoint is a single (timestamp, value) entry on a time-series chart.
type ChartPoint struct {
	Ts    time.Time
	Value float64
}

// ChartData holds equity-curve and drawdown series for a completed backtest.
// Each slice has one point per closed trade, keyed by ExitTimestamp.
// Equity values are cumulative PnL from zero; drawdown values are percentage
// decline from the running peak (0 = at peak, 6.8 = 6.8% below peak).
type ChartData struct {
	Equity   []ChartPoint
	Drawdown []ChartPoint
}

// BacktestResult aggregates the outcome of a full backtest run.
// GrossPnL is the sum of trade-level GrossPnL; TotalCharges is the sum of trade-level
// TotalCharges; NetPnL is the sum of trade-level NetPnL (== GrossPnL − TotalCharges
// within float-rounding tolerance). WinCount/LossCount are bucketed on NetPnL.
type BacktestResult struct {
	Trades       []Trade
	GrossPnL     float64
	TotalCharges float64
	NetPnL       float64
	TotalTrades  int
	WinCount     int
	LossCount    int
	MaxDrawdown  float64
}

// EngineInputs carries the strategy interval and candle streams consumed by the backtest engine.
type EngineInputs struct {
	Interval      string
	SignalCandles []Candle
	TradeCandles  []Candle
}

// BacktestEngine is the contract every backtest engine implementation must satisfy.
type BacktestEngine interface {
	// RunBacktest simulates strategy against the provided candle streams and returns
	// aggregated trade results. Candle streams must be in chronological order.
	// Signal candles generate entry and reversal decisions only. Trade candles are
	// the canonical execution stream for prices, exits, and mark-to-market. Instrument
	// metadata is resolved before the engine runs; v1 treats FUT*_CONT candles as a
	// synthetic continuous execution stream, while explicit roll events can be added to
	// EngineInputs later without exposing instrument taxonomy to the engine.
	// Entry decisions use an inner join by signal timestamp and trade timestamp.
	// A signal at T without a trade bar at T is skipped. Executions happen at the
	// open of the next trade bar starting at or after T plus the strategy interval.
	// Open positions evaluate exits on every trade-side bar, even when no signal
	// candle exists for that timestamp.
	// capital is the user's starting capital and is used as the drawdown denominator
	// when the equity curve has not yet gone positive (avoids division by zero).
	// Returns an error only if the strategy configuration is invalid.
	//
	// Each returned Trade carries a full charge breakdown (Slippage, Brokerage, STT,
	// ExchangeFees, SEBIFees, GST, StampDuty, TotalCharges) and both GrossPnL and
	// NetPnL = GrossPnL − TotalCharges. The aggregate BacktestResult mirrors this:
	// GrossPnL, TotalCharges, and NetPnL are summed from trade-level values, and the
	// equity curve and drawdown are driven by NetPnL (the post-charges series).
	RunBacktest(strategy *Strategy, inputs EngineInputs, capital float64) (*BacktestResult, error)
	// ComputeTradeStats derives performance statistics from a completed trade list.
	// from and to are the backtest date range used to compute tradesPerWeek.
	// Pointer fields in the result are nil when mathematically undefined.
	ComputeTradeStats(trades []Trade, from, to time.Time) *TradeStats
	// BuildChartData builds equity-curve and drawdown time series from a completed trade list.
	// Each point is keyed by the trade's ExitTimestamp.
	// capital is the user's starting capital and is used as the drawdown denominator
	// when the running equity peak has not yet gone positive.
	BuildChartData(trades []Trade, capital float64) *ChartData
}

// BacktestRepository is the storage contract for persisting backtest run records.
type BacktestRepository interface {
	// Create inserts a new BacktestRun record with PENDING status.
	Create(ctx context.Context, run *BacktestRun) error
	// UpdateStatus persists only the status field — used for PENDING→RUNNING transitions.
	UpdateStatus(ctx context.Context, run *BacktestRun) error
	// UpdateResult persists final metrics and stamps completed_at — used for COMPLETED/FAILED.
	UpdateResult(ctx context.Context, run *BacktestRun) error
	// GetByID returns the run with the given ID, or models.ErrNotFound.
	// Does not load the trades_json blob; use GetByIDWithTrades when trade data is needed.
	GetByID(ctx context.Context, id uuid.UUID) (*BacktestRun, error)
	// GetByIDWithTrades returns the run with the given ID including the trades_json blob, or models.ErrNotFound.
	// Use only when trade-level data is needed (e.g. the paginated trades endpoint).
	GetByIDWithTrades(ctx context.Context, id uuid.UUID) (*BacktestRun, error)
	// ListByStrategy returns all runs for a strategy, newest first.
	ListByStrategy(ctx context.Context, strategyID uuid.UUID) ([]BacktestRun, error)
	// LatestCompletedBySlug returns the most recent COMPLETED backtest for a built-in strategy slug.
	// Returns models.ErrNotFound if no completed run exists.
	LatestCompletedBySlug(ctx context.Context, slug string) (*BacktestRun, error)
	// ListCompleted returns completed backtest runs ordered by completed_at descending.
	ListCompleted(ctx context.Context, page, limit int) ([]BacktestRun, int, error)
}

// BacktestRun is the domain representation of a single backtest execution.
type BacktestRun struct {
	ID                    uuid.UUID
	StrategyID            *uuid.UUID
	InstrumentToken       string
	SignalInstrumentToken *string
	FromTs                time.Time
	ToTs                  time.Time
	CandleInterval        string
	Status                string
	NetPnl                *float64
	GrossPnl              *float64
	TotalCharges          *float64
	TotalTrades           *int
	WinCount              *int
	LossCount             *int
	MaxDrawdown           *float64
	Trades                []Trade
	ErrorMessage          *string
	CreatedAt             time.Time
	CompletedAt           *time.Time
	StrategySlug          *string
	StrategyName          *string // transient: populated by service from builtins registry, never persisted
	Capital               *float64
	Lots                  *int
	Underlying            *string
	ResultStats           *TradeStats
	ChartData             *ChartData
}

// Backtest run status values stored in the status column of backtest_runs.
const (
	BacktestPending   = "PENDING"
	BacktestRunning   = "RUNNING"
	BacktestCompleted = "COMPLETED"
	BacktestFailed    = "FAILED"
)

// TODO(persist-dto): legacy shim — delete after one major release once tradePersist.UnmarshalJSON
// in internal/storage handles all reads. See scratch/trades-json-persist-dto.md.
//
// UnmarshalJSON decodes a Trade with backward compatibility for pre-B14 records.
// Before B14, persisted trades_json used Go's default marshaling of the legacy
// Trade.PnL field, producing objects with a "PnL" key. Post-B14 the field was
// split into GrossPnL/NetPnL/TotalCharges and never marshals "PnL" again.
//
// When a legacy "PnL" key is present, we mirror it to both NetPnL and GrossPnL
// and leave the charge breakdown at zero. Detection is unambiguous because
// current code cannot emit "PnL" — so a non-nil legacy pointer always means a
// pre-B14 record, regardless of whether the new fields happen to be zero.
func (t *Trade) UnmarshalJSON(data []byte) error {
	type tradeAlias Trade
	aux := struct {
		*tradeAlias
		LegacyPnL *float64 `json:"PnL"`
	}{tradeAlias: (*tradeAlias)(t)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.LegacyPnL != nil {
		t.NetPnL = *aux.LegacyPnL
		t.GrossPnL = *aux.LegacyPnL
	}
	return nil
}
