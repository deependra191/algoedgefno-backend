package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type CandleFilter struct {
	InstrumentID uuid.UUID
	From         time.Time
	To           time.Time
	Interval     string
}

type CandleStore struct {
	pool *pgxpool.Pool
}

func NewCandleStore(pool *pgxpool.Pool) *CandleStore {
	return &CandleStore{pool: pool}
}

// InsertBatch bulk-inserts candles using the PostgreSQL COPY protocol for performance.
func (s *CandleStore) InsertBatch(ctx context.Context, candles []models.Candle) (int64, error) {
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

// Query returns candles for an instrument within a time range, ordered chronologically.
func (s *CandleStore) Query(ctx context.Context, f CandleFilter) ([]models.Candle, error) {
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
		var c models.Candle
		if err := rows.Scan(&c.InstrumentID, &c.Timestamp, &c.Interval,
			&c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.Provider); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
