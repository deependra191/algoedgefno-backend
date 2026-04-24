package models

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InstrumentType constants for the NSE F&O instrument classification codes.
const (
	InstrumentTypeFuturesIndex = "FUTIDX"
)

// Underlying constants for supported index underlyings.
const (
	UnderlyingNifty     = "NIFTY"
	UnderlyingBankNifty = "BANKNIFTY"
	UnderlyingFinNifty  = "FINNIFTY"
)

// InstrumentRepository is the storage contract for tradable instrument records.
type InstrumentRepository interface {
	// GetByID returns the instrument with the given ID, or models.ErrNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*Instrument, error)
	// List returns instruments matching the filter. All filter fields are optional.
	List(ctx context.Context, filter InstrumentFilter) ([]Instrument, error)
	// UpsertBatch inserts or updates instruments in bulk, keyed on (symbol, exchange).
	UpsertBatch(ctx context.Context, instruments []Instrument) error
}

// InstrumentFilter specifies optional constraints for listing instruments.
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
