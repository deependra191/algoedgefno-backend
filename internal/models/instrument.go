package models

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type InstrumentRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Instrument, error)
	List(ctx context.Context, filter InstrumentFilter) ([]Instrument, error)
	UpsertBatch(ctx context.Context, instruments []Instrument) error
}

type InstrumentFilter struct {
	Exchange       *string
	InstrumentType *string
	Underlying     *string
}

// Instrument is the domain representation of a tradable contract.
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
