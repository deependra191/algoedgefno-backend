package models

import (
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

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

func FromCandleEntity(e *entities.Candle) *Candle {
	if e == nil {
		return nil
	}
	return &Candle{
		InstrumentID: e.InstrumentID,
		Timestamp:    e.Timestamp,
		Interval:     e.Interval,
		Open:         e.Open,
		High:         e.High,
		Low:          e.Low,
		Close:        e.Close,
		Volume:       e.Volume,
		Provider:     e.Provider,
	}
}

func (c *Candle) ToEntity() *entities.Candle {
	if c == nil {
		return nil
	}
	return &entities.Candle{
		InstrumentID: c.InstrumentID,
		Timestamp:    c.Timestamp,
		Interval:     c.Interval,
		Open:         c.Open,
		High:         c.High,
		Low:          c.Low,
		Close:        c.Close,
		Volume:       c.Volume,
		Provider:     c.Provider,
	}
}
