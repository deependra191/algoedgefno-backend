package entities

import (
	"time"

	"github.com/google/uuid"
)

// BacktestRun is the DB row for the backtest_runs table.
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
	Slippage              *float64
	TotalTrades           *int
	WinCount              *int
	LossCount             *int
	MaxDrawdown           *float64
	// Populated only by scanBacktestRunWithTrades; consumed only by decodeTradesJSON. Mappers must not touch this field.
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
	SlippagePct     float64
}
