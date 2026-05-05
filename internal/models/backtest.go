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
type Trade struct {
	EntryTimestamp time.Time
	ExitTimestamp  time.Time
	Side           OrderSide
	Quantity       int
	EntryPrice     float64
	ExitPrice      float64
	PnL            float64
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
type BacktestResult struct {
	Trades      []Trade
	NetPnL      float64
	TotalTrades int
	WinCount    int
	LossCount   int
	MaxDrawdown float64
}

// BacktestEngine is the contract every backtest engine implementation must satisfy.
type BacktestEngine interface {
	// RunBacktest simulates strategy against the provided candle series and returns
	// aggregated trade results. Candles must be in chronological order.
	// capital is the user's starting capital and is used as the drawdown denominator
	// when the equity curve has not yet gone positive (avoids division by zero).
	// Returns an error only if the strategy configuration is invalid.
	RunBacktest(strategy *Strategy, candles []Candle, capital float64) (*BacktestResult, error)
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
	// ListAll returns all backtest runs ordered by created_at descending.
	ListAll(ctx context.Context) ([]BacktestRun, error)
}

// BacktestRun is the domain representation of a single backtest execution.
type BacktestRun struct {
	ID              uuid.UUID
	StrategyID      *uuid.UUID
	InstrumentToken string
	FromTs          time.Time
	ToTs            time.Time
	CandleInterval  string
	Status          string
	NetPnl          *float64
	TotalTrades     *int
	WinCount        *int
	LossCount       *int
	MaxDrawdown     *float64
	Trades          json.RawMessage
	ErrorMessage    *string
	CreatedAt       time.Time
	CompletedAt     *time.Time
	StrategySlug    *string
	Capital         *float64
	Lots            *int
	Underlying      *string
	ResultStats     *TradeStats
	ChartData       *ChartData
}

// Backtest run status values stored in the status column of backtest_runs.
const (
	BacktestPending   = "PENDING"
	BacktestRunning   = "RUNNING"
	BacktestCompleted = "COMPLETED"
	BacktestFailed    = "FAILED"
)
