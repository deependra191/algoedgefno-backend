package models

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CandleFilter specifies the query parameters for fetching candles from the repository.
type CandleFilter struct {
	InstrumentID uuid.UUID
	From         time.Time
	To           time.Time
	Interval     string
}

// CandleRepository is the storage contract for candle data.
type CandleRepository interface {
	// Query returns candles matching the filter, ordered chronologically.
	Query(ctx context.Context, f CandleFilter) ([]Candle, error)
	// InsertBatchIgnoreDuplicates bulk-inserts candles, skipping any that already
	// exist for the same (instrument_id, ts, interval). Returns the count inserted.
	InsertBatchIgnoreDuplicates(ctx context.Context, candles []Candle) (int64, error)
}

// Candle is the domain representation of an OHLCV bar.
type Candle struct {
	InstrumentID uuid.UUID
	Timestamp    time.Time
	Interval     string
	Open         float64
	High         float64
	Low          float64
	Close        float64
	Volume       int64
	Provider     string
}
