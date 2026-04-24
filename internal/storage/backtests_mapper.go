package storage

import (
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func toBacktestModel(e *entities.BacktestRun) *models.BacktestRun {
	return &models.BacktestRun{
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
		Trades:          json.RawMessage(e.TradesJSON),
		StrategySlug:    e.StrategySlug,
		Capital:         e.Capital,
		Lots:            e.Lots,
		Underlying:      e.Underlying,
	}
}

func toBacktestEntity(r *models.BacktestRun) *entities.BacktestRun {
	return &entities.BacktestRun{
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
		TradesJSON:      []byte(r.Trades),
		StrategySlug:    r.StrategySlug,
		Capital:         r.Capital,
		Lots:            r.Lots,
		Underlying:      r.Underlying,
	}
}

func scanBacktestRun(row pgx.Row) (*entities.BacktestRun, error) {
	var r entities.BacktestRun
	var tradesBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := row.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.FromTs, &r.ToTs,
		&r.CandleInterval, &r.Status,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		&tradesBytes, &errMsg, &r.CreatedAt, &r.CompletedAt,
		&r.StrategySlug, &r.Capital, &r.Lots, &r.Underlying,
	)
	if err != nil {
		return nil, err
	}
	r.TradesJSON = tradesBytes
	r.NetPnl = netPnl
	r.TotalTrades = totalTrades
	r.WinCount = winCount
	r.LossCount = lossCount
	r.MaxDrawdown = maxDrawdown
	r.ErrorMessage = errMsg
	return &r, nil
}

func scanBacktestRunRow(rows pgx.Rows) (*entities.BacktestRun, error) {
	var r entities.BacktestRun
	var tradesBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := rows.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.FromTs, &r.ToTs,
		&r.CandleInterval, &r.Status,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		&tradesBytes, &errMsg, &r.CreatedAt, &r.CompletedAt,
		&r.StrategySlug, &r.Capital, &r.Lots, &r.Underlying,
	)
	if err != nil {
		return nil, err
	}
	r.TradesJSON = tradesBytes
	r.NetPnl = netPnl
	r.TotalTrades = totalTrades
	r.WinCount = winCount
	r.LossCount = lossCount
	r.MaxDrawdown = maxDrawdown
	r.ErrorMessage = errMsg
	return &r, nil
}
