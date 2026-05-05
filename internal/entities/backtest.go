package entities

import (
	"time"

	"github.com/google/uuid"
)

// BacktestRun is the DB row for the backtest_runs table.
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
	TradesJSON      []byte
	ErrorMessage    *string
	CreatedAt       time.Time
	CompletedAt     *time.Time
	StrategySlug    *string
	Capital         *float64
	Lots            *int
	Underlying      *string
	ResultStatsJSON []byte
	ChartDataJSON   []byte
}
