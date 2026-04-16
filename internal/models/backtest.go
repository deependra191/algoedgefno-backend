package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

// BacktestRun is the domain representation of a backtest execution.
// TradesJSON is a temporary bridge until the engine defines a typed Trade
// struct in B09; at that point TradesJSON is replaced by []engine.Trade.
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
	TradesJSON      *json.RawMessage
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

func FromBacktestRunEntity(e *entities.BacktestRun) *BacktestRun {
	if e == nil {
		return nil
	}
	r := &BacktestRun{
		ID:              e.ID,
		StrategyID:      e.StrategyID,
		InstrumentToken: e.InstrumentToken,
		FromTs:          e.FromTs,
		ToTs:            e.ToTs,
		CandleInterval:  e.CandleInterval,
		Status:          e.Status,
		NetPnl:          e.NetPnl,
		TotalTrades:     e.TotalTrades,
		WinCount:        e.WinCount,
		LossCount:       e.LossCount,
		MaxDrawdown:     e.MaxDrawdown,
		ErrorMessage:    e.ErrorMessage,
		CreatedAt:       e.CreatedAt,
		CompletedAt:     e.CompletedAt,
	}
	if e.TradesJSON != nil {
		raw := json.RawMessage(e.TradesJSON)
		r.TradesJSON = &raw
	}
	return r
}

func (r *BacktestRun) ToEntity() *entities.BacktestRun {
	if r == nil {
		return nil
	}
	e := &entities.BacktestRun{
		ID:              r.ID,
		StrategyID:      r.StrategyID,
		InstrumentToken: r.InstrumentToken,
		FromTs:          r.FromTs,
		ToTs:            r.ToTs,
		CandleInterval:  r.CandleInterval,
		Status:          r.Status,
		NetPnl:          r.NetPnl,
		TotalTrades:     r.TotalTrades,
		WinCount:        r.WinCount,
		LossCount:       r.LossCount,
		MaxDrawdown:     r.MaxDrawdown,
		ErrorMessage:    r.ErrorMessage,
		CreatedAt:       r.CreatedAt,
		CompletedAt:     r.CompletedAt,
	}
	if r.TradesJSON != nil {
		e.TradesJSON = []byte(*r.TradesJSON)
	}
	return e
}
