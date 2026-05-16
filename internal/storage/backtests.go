package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
	if len(run.Trades) > 0 {
		b, err := encodeTradesJSON(run.Trades)
		if err != nil {
			return fmt.Errorf("encoding trades_json for backtest %s: %w", run.ID, err)
		}
		tradesJSON = string(b)
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
			gross_pnl     = $12,
			total_charges = $13,
			completed_at  = NOW()
		WHERE id = $1`,
		ent.ID, ent.Status, ent.NetPnl, ent.TotalTrades, ent.WinCount,
		ent.LossCount, ent.MaxDrawdown, tradesJSON, ent.ErrorMessage,
		resultStatsJSON, chartDataJSON,
		ent.GrossPnl, ent.TotalCharges,
	)
	return err
}

func (s *BacktestStore) GetByID(ctx context.Context, id uuid.UUID) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying,
		       result_stats, chart_data,
		       gross_pnl, total_charges
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
		       strategy_slug, capital, lots, underlying,
		       gross_pnl, total_charges
		FROM backtest_runs WHERE id = $1`, id)
	ent, err := scanBacktestRunWithTrades(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, models.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	run := toBacktestModel(ent)
	if len(ent.TradesJSON) > 0 {
		trades, err := decodeTradesJSON(ent.TradesJSON)
		if err != nil {
			return nil, fmt.Errorf("decoding trades_json for backtest %s: %w", id, err)
		}
		run.Trades = trades
	}
	return run, nil
}

// ListCompleted implements BacktestRepository. The total_trades > 0 predicate is
// repeated in both the COUNT and the paginated SELECT — they must stay in sync,
// otherwise the returned total and the page contents disagree and pagination breaks.
func (s *BacktestStore) ListCompleted(ctx context.Context, page, limit int) ([]models.BacktestRun, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM backtest_runs
		WHERE status = $1 AND completed_at IS NOT NULL AND total_trades > 0`, models.BacktestCompleted).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	rows, err := s.pool.Query(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying,
		       gross_pnl, total_charges
		FROM backtest_runs
		WHERE status = $1 AND completed_at IS NOT NULL AND total_trades > 0
		ORDER BY completed_at DESC, created_at DESC
		LIMIT $2 OFFSET $3`, models.BacktestCompleted, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []models.BacktestRun
	for rows.Next() {
		ent, err := scanBacktestRunRow(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, *toBacktestModel(ent))
	}
	return result, total, rows.Err()
}

// LatestCompletedBySlug returns the most recent COMPLETED backtest for a built-in strategy slug.
func (s *BacktestStore) LatestCompletedBySlug(ctx context.Context, slug string) (*models.BacktestRun, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, strategy_id, instrument_token, signal_instrument_token, from_ts, to_ts, candle_interval, status,
		       net_pnl, total_trades, win_count, loss_count, max_drawdown,
		       error_message, created_at, completed_at,
		       strategy_slug, capital, lots, underlying,
		       result_stats, chart_data,
		       gross_pnl, total_charges
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
