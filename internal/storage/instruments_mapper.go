package storage

import (
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func toInstrumentModel(e *entities.Instrument) *models.Instrument {
	return &models.Instrument{
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

func scanInstrument(row pgx.Row) (*entities.Instrument, error) {
	var inst entities.Instrument
	var underlying *string
	var expiry *time.Time
	var strike *float64
	var optionType *string
	err := row.Scan(
		&inst.ID, &inst.Symbol, &inst.Name, &inst.Exchange, &inst.InstrumentType,
		&underlying, &expiry, &strike, &optionType,
		&inst.LotSize, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	inst.Underlying = underlying
	inst.Expiry = expiry
	inst.Strike = strike
	inst.OptionType = optionType
	return &inst, nil
}

func scanInstrumentRow(rows pgx.Rows) (*entities.Instrument, error) {
	var inst entities.Instrument
	var underlying *string
	var expiry *time.Time
	var strike *float64
	var optionType *string
	err := rows.Scan(
		&inst.ID, &inst.Symbol, &inst.Name, &inst.Exchange, &inst.InstrumentType,
		&underlying, &expiry, &strike, &optionType,
		&inst.LotSize, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	inst.Underlying = underlying
	inst.Expiry = expiry
	inst.Strike = strike
	inst.OptionType = optionType
	return &inst, nil
}
