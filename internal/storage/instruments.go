package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type InstrumentFilter struct {
	Exchange       *string
	InstrumentType *string
	Underlying     *string
}

type InstrumentStore struct {
	pool *pgxpool.Pool
}

func NewInstrumentStore(pool *pgxpool.Pool) *InstrumentStore {
	return &InstrumentStore{pool: pool}
}

func (s *InstrumentStore) Upsert(ctx context.Context, inst *models.Instrument) error {
	if inst.ID == uuid.Nil {
		inst.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO instruments
			(id, symbol, name, exchange, instrument_type, underlying, expiry, strike, option_type, lot_size, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (symbol, exchange) DO UPDATE SET
			name            = EXCLUDED.name,
			instrument_type = EXCLUDED.instrument_type,
			underlying      = EXCLUDED.underlying,
			expiry          = EXCLUDED.expiry,
			strike          = EXCLUDED.strike,
			option_type     = EXCLUDED.option_type,
			lot_size        = EXCLUDED.lot_size,
			updated_at      = NOW()`,
		inst.ID, inst.Symbol, inst.Name, inst.Exchange, inst.InstrumentType,
		inst.Underlying, inst.Expiry, inst.Strike, inst.OptionType, inst.LotSize,
	)
	return err
}

func (s *InstrumentStore) UpsertBatch(ctx context.Context, instruments []models.Instrument) error {
	if len(instruments) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for i := range instruments {
		inst := &instruments[i]
		if inst.ID == uuid.Nil {
			inst.ID = uuid.New()
		}
		batch.Queue(`
			INSERT INTO instruments
				(id, symbol, name, exchange, instrument_type, underlying, expiry, strike, option_type, lot_size, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
			ON CONFLICT (symbol, exchange) DO UPDATE SET
				name            = EXCLUDED.name,
				instrument_type = EXCLUDED.instrument_type,
				underlying      = EXCLUDED.underlying,
				expiry          = EXCLUDED.expiry,
				strike          = EXCLUDED.strike,
				option_type     = EXCLUDED.option_type,
				lot_size        = EXCLUDED.lot_size,
				updated_at      = NOW()`,
			inst.ID, inst.Symbol, inst.Name, inst.Exchange, inst.InstrumentType,
			inst.Underlying, inst.Expiry, inst.Strike, inst.OptionType, inst.LotSize,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range instruments {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *InstrumentStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Instrument, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, symbol, name, exchange, instrument_type, underlying, expiry, strike, option_type, lot_size, created_at, updated_at
		FROM instruments WHERE id = $1`, id)
	return scanInstrument(row)
}

func (s *InstrumentStore) List(ctx context.Context, filter InstrumentFilter) ([]models.Instrument, error) {
	q := `SELECT id, symbol, name, exchange, instrument_type, underlying, expiry, strike, option_type, lot_size, created_at, updated_at FROM instruments`
	var conds []string
	var args []any
	idx := 1

	if filter.Exchange != nil {
		conds = append(conds, fmt.Sprintf("exchange = $%d", idx))
		args = append(args, *filter.Exchange)
		idx++
	}
	if filter.InstrumentType != nil {
		conds = append(conds, fmt.Sprintf("instrument_type = $%d", idx))
		args = append(args, *filter.InstrumentType)
		idx++
	}
	if filter.Underlying != nil {
		conds = append(conds, fmt.Sprintf("underlying = $%d", idx))
		args = append(args, *filter.Underlying)
		idx++
	}
	_ = idx // suppress unused warning if no filters
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY symbol ASC"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Instrument
	for rows.Next() {
		inst, err := scanInstrumentRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *inst)
	}
	return result, rows.Err()
}

// scanInstrument scans a single pgx.Row (QueryRow result).
func scanInstrument(row pgx.Row) (*models.Instrument, error) {
	var inst models.Instrument
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

// scanInstrumentRow scans from pgx.Rows (Query result).
func scanInstrumentRow(rows pgx.Rows) (*models.Instrument, error) {
	var inst models.Instrument
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
