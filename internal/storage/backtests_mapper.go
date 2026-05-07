package storage

import (
	"encoding/json"
	"log"

	"github.com/jackc/pgx/v5"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func toBacktestModel(e *entities.BacktestRun) *models.BacktestRun {
	run := &models.BacktestRun{
		ID:                    e.ID,
		StrategyID:            e.StrategyID,
		InstrumentToken:       e.InstrumentToken,
		SignalInstrumentToken: e.SignalInstrumentToken,
		FromTs:                e.FromTs,
		ToTs:                  e.ToTs,
		CandleInterval:        e.CandleInterval,
		Status:                e.Status,
		NetPnl:                e.NetPnl,
		TotalTrades:           e.TotalTrades,
		WinCount:              e.WinCount,
		LossCount:             e.LossCount,
		MaxDrawdown:           e.MaxDrawdown,
		ErrorMessage:          e.ErrorMessage,
		CreatedAt:             e.CreatedAt,
		CompletedAt:           e.CompletedAt,
		Trades:                json.RawMessage(e.TradesJSON),
		StrategySlug:          e.StrategySlug,
		Capital:               e.Capital,
		Lots:                  e.Lots,
		Underlying:            e.Underlying,
	}
	if len(e.ResultStatsJSON) > 0 {
		var stats models.TradeStats
		if err := json.Unmarshal(e.ResultStatsJSON, &stats); err != nil {
			log.Printf("backtest %s: corrupt result_stats JSON: %v", e.ID, err)
		} else {
			run.ResultStats = &stats
		}
	}
	if len(e.ChartDataJSON) > 0 {
		var cd models.ChartData
		if err := json.Unmarshal(e.ChartDataJSON, &cd); err != nil {
			log.Printf("backtest %s: corrupt chart_data JSON: %v", e.ID, err)
		} else {
			run.ChartData = &cd
		}
	}
	return run
}

func toBacktestEntity(r *models.BacktestRun) *entities.BacktestRun {
	ent := &entities.BacktestRun{
		ID:                    r.ID,
		StrategyID:            r.StrategyID,
		InstrumentToken:       r.InstrumentToken,
		SignalInstrumentToken: r.SignalInstrumentToken,
		FromTs:                r.FromTs,
		ToTs:                  r.ToTs,
		CandleInterval:        r.CandleInterval,
		Status:                r.Status,
		NetPnl:                r.NetPnl,
		TotalTrades:           r.TotalTrades,
		WinCount:              r.WinCount,
		LossCount:             r.LossCount,
		MaxDrawdown:           r.MaxDrawdown,
		ErrorMessage:          r.ErrorMessage,
		CreatedAt:             r.CreatedAt,
		CompletedAt:           r.CompletedAt,
		TradesJSON:            []byte(r.Trades),
		StrategySlug:          r.StrategySlug,
		Capital:               r.Capital,
		Lots:                  r.Lots,
		Underlying:            r.Underlying,
	}
	if r.ResultStats != nil {
		if b, err := json.Marshal(r.ResultStats); err == nil {
			ent.ResultStatsJSON = b
		}
	}
	if r.ChartData != nil {
		if b, err := json.Marshal(r.ChartData); err == nil {
			ent.ChartDataJSON = b
		}
	}
	return ent
}

// scanBacktestRun scans a single row from GetByID and LatestCompletedBySlug,
// which SELECT result_stats and chart_data but not trades_json.
func scanBacktestRun(row pgx.Row) (*entities.BacktestRun, error) {
	var r entities.BacktestRun
	var resultStatsBytes, chartDataBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := row.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.SignalInstrumentToken, &r.FromTs, &r.ToTs,
		&r.CandleInterval, &r.Status,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		&errMsg, &r.CreatedAt, &r.CompletedAt,
		&r.StrategySlug, &r.Capital, &r.Lots, &r.Underlying,
		&resultStatsBytes, &chartDataBytes,
	)
	if err != nil {
		return nil, err
	}
	r.NetPnl = netPnl
	r.TotalTrades = totalTrades
	r.WinCount = winCount
	r.LossCount = lossCount
	r.MaxDrawdown = maxDrawdown
	r.ErrorMessage = errMsg
	r.ResultStatsJSON = resultStatsBytes
	r.ChartDataJSON = chartDataBytes
	return &r, nil
}

// scanBacktestRunWithTrades scans a single row from GetByIDWithTrades,
// which SELECTs trades_json but not result_stats or chart_data.
func scanBacktestRunWithTrades(row pgx.Row) (*entities.BacktestRun, error) {
	var r entities.BacktestRun
	var tradesBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := row.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.SignalInstrumentToken, &r.FromTs, &r.ToTs,
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

// scanBacktestRunRow scans a row from list queries, which do not SELECT result_stats or chart_data.
func scanBacktestRunRow(rows pgx.Rows) (*entities.BacktestRun, error) {
	var r entities.BacktestRun
	var tradesBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := rows.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.SignalInstrumentToken, &r.FromTs, &r.ToTs,
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
