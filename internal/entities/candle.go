package entities

import (
	"time"

	"github.com/google/uuid"
)

// Candle is the DB row for the candles hypertable.
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
