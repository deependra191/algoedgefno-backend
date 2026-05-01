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
	// Returns an error only if the strategy configuration is invalid.
	RunBacktest(strategy *Strategy, candles []Candle) (*BacktestResult, error)
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
	GetByID(ctx context.Context, id uuid.UUID) (*BacktestRun, error)
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
}

const (
	BacktestPending   = "PENDING"
	BacktestRunning   = "RUNNING"
	BacktestCompleted = "COMPLETED"
	BacktestFailed    = "FAILED"
)
