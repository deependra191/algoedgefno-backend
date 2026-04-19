package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.CandleRepository = (*CandleStore)(nil)

type CandleStore struct {
	pool *pgxpool.Pool
}

func NewCandleStore(pool *pgxpool.Pool) *CandleStore {
	return &CandleStore{pool: pool}
}

// InsertBatch bulk-inserts candles using the PostgreSQL COPY protocol for performance.
// Fails on duplicate (instrument_id, ts, interval) — caller must ensure no duplicates exist.
// For re-sync scenarios, delete existing data first, then re-insert.
func (s *CandleStore) InsertBatch(ctx context.Context, candles []entities.Candle) (int64, error) {
	if len(candles) == 0 {
		return 0, nil
	}
	columns := []string{"instrument_id", "ts", "interval", "open", "high", "low", "close", "volume", "provider"}
	count, err := s.pool.CopyFrom(
		ctx,
		pgx.Identifier{"candles"},
		columns,
		pgx.CopyFromSlice(len(candles), func(i int) ([]any, error) {
			c := candles[i]
			return []any{c.InstrumentID, c.Timestamp, c.Interval, c.Open, c.High, c.Low, c.Close, c.Volume, c.Provider}, nil
		}),
	)
	return count, err
}

// InsertBatchIgnoreDuplicates bulk-inserts candles, skipping any that already exist
// (matching on the instrument_id, ts, interval primary key).
// Uses COPY into a staging table + INSERT ... ON CONFLICT DO NOTHING for performance.
func (s *CandleStore) InsertBatchIgnoreDuplicates(ctx context.Context, candles []entities.Candle) (int64, error) {
	if len(candles) == 0 {
		return 0, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE _candle_stage (
			instrument_id UUID,
			ts            TIMESTAMPTZ,
			interval      TEXT,
			open          DOUBLE PRECISION,
			high          DOUBLE PRECISION,
			low           DOUBLE PRECISION,
			close         DOUBLE PRECISION,
			volume        BIGINT,
			provider      TEXT
		) ON COMMIT DROP`)
	if err != nil {
		return 0, fmt.Errorf("create staging table: %w", err)
	}

	columns := []string{"instrument_id", "ts", "interval", "open", "high", "low", "close", "volume", "provider"}
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"_candle_stage"},
		columns,
		pgx.CopyFromSlice(len(candles), func(i int) ([]any, error) {
			c := candles[i]
			return []any{c.InstrumentID, c.Timestamp, c.Interval, c.Open, c.High, c.Low, c.Close, c.Volume, c.Provider}, nil
		}),
	)
	if err != nil {
		return 0, fmt.Errorf("copy to staging: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO candles (instrument_id, ts, interval, open, high, low, close, volume, provider)
		SELECT instrument_id, ts, interval, open, high, low, close, volume, provider
		FROM _candle_stage
		ON CONFLICT (instrument_id, ts, interval) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("insert from staging: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

// LastSyncedDate returns the most recent candle date for a given provider.
// Returns a zero time.Time if no candles exist yet for that provider.
func (s *CandleStore) LastSyncedDate(ctx context.Context, provider string) (time.Time, error) {
	var d pgtype.Date
	err := s.pool.QueryRow(ctx,
		`SELECT MAX(ts::date) FROM candles WHERE provider = $1`,
		provider,
	).Scan(&d)
	if err != nil {
		return time.Time{}, err
	}
	if !d.Valid {
		return time.Time{}, nil
	}
	return d.Time, nil
}

// Query returns candles for an instrument within a time range, ordered chronologically.
func (s *CandleStore) Query(ctx context.Context, f models.CandleFilter) ([]models.Candle, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT instrument_id, ts, interval, open, high, low, close, volume, provider
		FROM candles
		WHERE instrument_id = $1 AND interval = $2 AND ts >= $3 AND ts <= $4
		ORDER BY ts ASC`,
		f.InstrumentID, f.Interval, f.From, f.To,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Candle
	for rows.Next() {
		var e entities.Candle
		if err := rows.Scan(&e.InstrumentID, &e.Timestamp, &e.Interval,
			&e.Open, &e.High, &e.Low, &e.Close, &e.Volume, &e.Provider); err != nil {
			return nil, err
		}
		result = append(result, toCandleModel(&e))
	}
	return result, rows.Err()
}

