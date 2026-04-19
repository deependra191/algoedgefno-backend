package storage

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.BacktestRepository = (*BacktestStore)(nil)

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
	ent := toBacktestEntity(run)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backtest_runs
			(id, strategy_id, instrument_token, from_ts, to_ts, candle_interval, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		ent.ID, ent.StrategyID, ent.InstrumentToken,
		ent.FromTs, ent.ToTs, ent.CandleInterval, ent.Status,
	)
	return err
}

// UpdateStatus persists only the status column — used for PENDING→RUNNING transitions.
func (s *BacktestStore) UpdateStatus(ctx context.Context, run *models.BacktestRun) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE backtest_runs SET status = $2 WHERE id = $1`,
		run.ID, run.Status,
	)
	return err
}

// UpdateResult persists final metrics and stamps completed_at — used for COMPLETED/FAILED.
func (s *BacktestStore) UpdateResult(ctx context.Context, run *models.BacktestRun) error {
	ent := toBacktestEntity(run)
	var tradesJSON any
	if ent.TradesJSON != nil {
		tradesJSON = string(ent.TradesJSON)
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
		ent.ID, ent.Status, ent.NetPnl, ent.TotalTrades, ent.WinCount,
		ent.LossCount, ent.MaxDrawdown, tradesJSON, ent.ErrorMessage,
	)
	return err
}

func (s *BacktestStore) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       trades_json, error_message, created_at, completed_at
		FROM backtest_runs WHERE id = $1`, id)
	ent, err := scanBacktestRun(row)
	if err != nil {
		return nil, err
	}
	return toBacktestModel(ent), nil
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
		ent, err := scanBacktestRunRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *toBacktestModel(ent))
	}
	return result, rows.Err()
}

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
