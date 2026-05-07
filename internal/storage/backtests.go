package storage

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const defaultListLimit = 100

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
			(id, strategy_id, strategy_slug, instrument_token, signal_instrument_token, from_ts, to_ts,
			 candle_interval, status, capital, lots, underlying, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())`,
		ent.ID, ent.StrategyID, ent.StrategySlug, ent.InstrumentToken, ent.SignalInstrumentToken,
		ent.FromTs, ent.ToTs, ent.CandleInterval, ent.Status,
		ent.Capital, ent.Lots, ent.Underlying,
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
	var tradesJSON, resultStatsJSON, chartDataJSON any
	if len(ent.TradesJSON) > 0 {
		tradesJSON = string(ent.TradesJSON)
	}
	if len(ent.ResultStatsJSON) > 0 {
		resultStatsJSON = string(ent.ResultStatsJSON)
	}
	if len(ent.ChartDataJSON) > 0 {
		chartDataJSON = string(ent.ChartDataJSON)
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
			result_stats  = $10::jsonb,
			chart_data    = $11::jsonb,
			completed_at  = NOW()
		WHERE id = $1`,
		ent.ID, ent.Status, ent.NetPnl, ent.TotalTrades, ent.WinCount,
		ent.LossCount, ent.MaxDrawdown, tradesJSON, ent.ErrorMessage,
		resultStatsJSON, chartDataJSON,
	)
	return err
}

func (s *BacktestStore) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying,
		       result_stats, chart_data
		FROM backtest_runs WHERE id = $1`, id)
	ent, err := scanBacktestRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toBacktestModel(ent), nil
}

// GetByIDWithTrades returns the run including the trades_json blob — use only for the trades endpoint.
func (s *BacktestStore) GetByIDWithTrades(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       trades_json, error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying
		FROM backtest_runs WHERE id = $1`, id)
	ent, err := scanBacktestRunWithTrades(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toBacktestModel(ent), nil
}

func (s *BacktestStore) ListByStrategy(ctx context.Context, strategyID uuid.UUID) ([]models.BacktestRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       trades_json, error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying
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

// ListAll returns the most recent backtest runs, capped at 100 rows.
func (s *BacktestStore) ListAll(ctx context.Context) ([]models.BacktestRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       trades_json, error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying
		FROM backtest_runs ORDER BY created_at DESC LIMIT $1`, defaultListLimit)
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

// LatestCompletedBySlug returns the most recent COMPLETED backtest for a built-in strategy slug.
func (s *BacktestStore) LatestCompletedBySlug(ctx context.Context, slug string) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying,
		       result_stats, chart_data
		FROM backtest_runs
		WHERE strategy_slug = $1 AND status = $2
		ORDER BY created_at DESC LIMIT 1`, slug, models.BacktestCompleted)
	ent, err := scanBacktestRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toBacktestModel(ent), nil
}
