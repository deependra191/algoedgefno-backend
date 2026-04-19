package models

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CandleFilter struct {
	InstrumentID uuid.UUID
	From         time.Time
	To           time.Time
	Interval     string
}

type CandleRepository interface {
	Query(ctx context.Context, f CandleFilter) ([]Candle, error)
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
