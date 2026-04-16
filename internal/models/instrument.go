package models

import (
	"time"

	"github.com/google/uuid"
)

type Instrument struct {
	ID             uuid.UUID  `json:"id"`
	Symbol         string     `json:"symbol"`
	Name           string     `json:"name"`
	Exchange       string     `json:"exchange"`
	InstrumentType string     `json:"instrument_type"`
	Underlying     *string    `json:"underlying,omitempty"`
	Expiry         *time.Time `json:"expiry,omitempty"`
	Strike         *float64   `json:"strike,omitempty"`
	OptionType     *string    `json:"option_type,omitempty"`
	LotSize        int        `json:"lot_size"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
