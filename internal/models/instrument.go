package models

import (
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

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

func FromInstrumentEntity(e *entities.Instrument) *Instrument {
	if e == nil {
		return nil
	}
	return &Instrument{
		ID:             e.ID,
		Symbol:         e.Symbol,
		Name:           e.Name,
		Exchange:       e.Exchange,
		InstrumentType: e.InstrumentType,
		Underlying:     e.Underlying,
		Expiry:         e.Expiry,
		Strike:         e.Strike,
		OptionType:     e.OptionType,
		LotSize:        e.LotSize,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

func (i *Instrument) ToEntity() *entities.Instrument {
	if i == nil {
		return nil
	}
	return &entities.Instrument{
		ID:             i.ID,
		Symbol:         i.Symbol,
		Name:           i.Name,
		Exchange:       i.Exchange,
		InstrumentType: i.InstrumentType,
		Underlying:     i.Underlying,
		Expiry:         i.Expiry,
		Strike:         i.Strike,
		OptionType:     i.OptionType,
		LotSize:        i.LotSize,
		CreatedAt:      i.CreatedAt,
		UpdatedAt:      i.UpdatedAt,
	}
}
