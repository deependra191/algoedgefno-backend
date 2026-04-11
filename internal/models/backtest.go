package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type BacktestRun struct {
	ID              uuid.UUID        `json:"id"`
	StrategyID      uuid.UUID        `json:"strategy_id"`
	InstrumentToken string           `json:"instrument_token"`
	FromTs          time.Time        `json:"from_ts"`
	ToTs            time.Time        `json:"to_ts"`
	CandleInterval  string           `json:"candle_interval"`
	Status          string           `json:"status"`
	NetPnl          *float64         `json:"net_pnl,omitempty"`
	TotalTrades     *int             `json:"total_trades,omitempty"`
	WinCount        *int             `json:"win_count,omitempty"`
	LossCount       *int             `json:"loss_count,omitempty"`
	MaxDrawdown     *float64         `json:"max_drawdown,omitempty"`
	TradesJSON      *json.RawMessage `json:"trades,omitempty"`
	ErrorMessage    *string          `json:"error_message,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty"`
}

const (
	BacktestPending   = "PENDING"
	BacktestRunning   = "RUNNING"
	BacktestCompleted = "COMPLETED"
	BacktestFailed    = "FAILED"
)
