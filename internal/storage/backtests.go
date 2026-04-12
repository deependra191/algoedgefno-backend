package storage

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type BacktestStore struct {
	pool *pgxpool.Pool
}

func NewBacktestStore(pool *pgxpool.Pool) *BacktestStore {
	return &BacktestStore{pool: pool}
}

func (s *BacktestStore) Create(ctx context.Context, run *models.BacktestRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backtest_runs
			(id, strategy_id, instrument_token, from_ts, to_ts, candle_interval, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		run.ID, run.StrategyID, run.InstrumentToken,
		run.FromTs, run.ToTs, run.CandleInterval, run.Status,
	)
	return err
}

func (s *BacktestStore) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       trades_json, error_message, created_at, completed_at
		FROM backtest_runs WHERE id = $1`, id)
	return scanBacktestRun(row)
}

func (s *BacktestStore) ListByStrategy(ctx context.Context, strategyID uuid.UUID) ([]models.BacktestRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, strategy_id, instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       trades_json, error_message, created_at, completed_at
		FROM backtest_runs WHERE strategy_id = $1 ORDER BY created_at DESC`, strategyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.BacktestRun
	for rows.Next() {
		run, err := scanBacktestRunRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *run)
	}
	return result, rows.Err()
}

// UpdateResult persists final backtest results (status, metrics, trades, error).
func (s *BacktestStore) UpdateResult(ctx context.Context, run *models.BacktestRun) error {
	var tradesJSON interface{}
	if run.TradesJSON != nil {
		tradesJSON = string(*run.TradesJSON)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE backtest_runs SET
			status        = $2,
			net_pnl       = $3,
			total_trades  = $4,
			win_count     = $5,
			loss_count    = $6,
			max_drawdown  = $7,
			trades_json   = $8::jsonb,
			error_message = $9,
			completed_at  = NOW()
		WHERE id = $1`,
		run.ID, run.Status, run.NetPnl, run.TotalTrades, run.WinCount,
		run.LossCount, run.MaxDrawdown, tradesJSON, run.ErrorMessage,
	)
	return err
}

func scanBacktestRun(row pgx.Row) (*models.BacktestRun, error) {
	var r models.BacktestRun
	var tradesBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := row.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.FromTs, &r.ToTs,
		&r.CandleInterval, &r.Status,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		&tradesBytes, &errMsg, &r.CreatedAt, &r.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	if tradesBytes != nil {
		raw := json.RawMessage(tradesBytes)
		r.TradesJSON = &raw
	}
	r.NetPnl = netPnl
	r.TotalTrades = totalTrades
	r.WinCount = winCount
	r.LossCount = lossCount
	r.MaxDrawdown = maxDrawdown
	r.ErrorMessage = errMsg
	return &r, nil
}

func scanBacktestRunRow(rows pgx.Rows) (*models.BacktestRun, error) {
	var r models.BacktestRun
	var tradesBytes []byte
	var netPnl, maxDrawdown *float64
	var totalTrades, winCount, lossCount *int
	var errMsg *string
	err := rows.Scan(
		&r.ID, &r.StrategyID, &r.InstrumentToken, &r.FromTs, &r.ToTs,
		&r.CandleInterval, &r.Status,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		&tradesBytes, &errMsg, &r.CreatedAt, &r.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	if tradesBytes != nil {
		raw := json.RawMessage(tradesBytes)
		r.TradesJSON = &raw
	}
	r.NetPnl = netPnl
	r.TotalTrades = totalTrades
	r.WinCount = winCount
	r.LossCount = lossCount
	r.MaxDrawdown = maxDrawdown
	r.ErrorMessage = errMsg
	return &r, nil
}
