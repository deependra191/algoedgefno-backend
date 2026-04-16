package entities

import (
	"time"

	"github.com/google/uuid"
)

// Instrument is the DB row for the instruments table.
type Instrument struct {
	ID             uuid.UUID
	Symbol         string
	Name           string
	Exchange       string
	InstrumentType string
	Underlying     *string
	Expiry         *time.Time
	Strike         *float64
	OptionType     *string
	LotSize        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
