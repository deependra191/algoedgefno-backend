package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

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
	RunBacktest(strategy *Strategy, candles []Candle) (*BacktestResult, error)
}

// BacktestRepository is the contract every backtest storage implementation must satisfy.
type BacktestRepository interface {
	Create(ctx context.Context, run *BacktestRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*BacktestRun, error)
	ListByStrategy(ctx context.Context, strategyID uuid.UUID) ([]BacktestRun, error)
	UpdateResult(ctx context.Context, run *BacktestRun) error
}

type BacktestRun struct {
	ID              uuid.UUID
	StrategyID      uuid.UUID
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
}

const (
	BacktestPending   = "PENDING"
	BacktestRunning   = "RUNNING"
	BacktestCompleted = "COMPLETED"
	BacktestFailed    = "FAILED"
)
