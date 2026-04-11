package models

import (
	"time"

	"github.com/google/uuid"
)

type Candle struct {
	InstrumentID uuid.UUID `json:"instrument_id"`
	Timestamp    time.Time `json:"ts"`
	Interval     string    `json:"interval"`
	Open         float64   `json:"open"`
	High         float64   `json:"high"`
	Low          float64   `json:"low"`
	Close        float64   `json:"close"`
	Volume       int64     `json:"volume"`
	Provider     string    `json:"provider"`
}
