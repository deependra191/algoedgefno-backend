package storage

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

type StrategyStore struct {
	pool *pgxpool.Pool
}

func NewStrategyStore(pool *pgxpool.Pool) *StrategyStore {
	return &StrategyStore{pool: pool}
}

func (s *StrategyStore) Create(ctx context.Context, strategy *entities.Strategy) error {
	if strategy.ID == uuid.Nil {
		strategy.ID = uuid.New()
	}
	var optionLeg interface{}
	if strategy.OptionLegJSON != nil {
		optionLeg = string(strategy.OptionLegJSON)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO strategies
			(id, name, description, underlying, instrument_type, expiry_rule, option_leg_json,
			 entry_condition_type, target_pct, stop_loss_pct, time_exit_minutes, lot_size,
			 capital_per_trade, mode, is_ready_for_run, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13,$14,$15,NOW(),NOW())`,
		strategy.ID, strategy.Name, strategy.Description, strategy.Underlying,
		strategy.InstrumentType, strategy.ExpiryRule, optionLeg,
		strategy.EntryConditionType, strategy.TargetPct, strategy.StopLossPct,
		strategy.TimeExitMinutes, strategy.LotSize, strategy.CapitalPerTrade,
		strategy.Mode, strategy.IsReadyForRun,
	)
	return err
}

func (s *StrategyStore) GetByID(ctx context.Context, id uuid.UUID) (*entities.Strategy, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, description, underlying, instrument_type, expiry_rule, option_leg_json,
		       entry_condition_type, target_pct, stop_loss_pct, time_exit_minutes, lot_size,
		       capital_per_trade, mode, is_ready_for_run, created_at, updated_at
		FROM strategies WHERE id = $1`, id)
	return scanStrategy(row)
}

func (s *StrategyStore) List(ctx context.Context) ([]entities.Strategy, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, underlying, instrument_type, expiry_rule, option_leg_json,
		       entry_condition_type, target_pct, stop_loss_pct, time_exit_minutes, lot_size,
		       capital_per_trade, mode, is_ready_for_run, created_at, updated_at
		FROM strategies ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []entities.Strategy
	for rows.Next() {
		s, err := scanStrategyRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}
	return result, rows.Err()
}

func (s *StrategyStore) Update(ctx context.Context, strategy *entities.Strategy) error {
	var optionLeg interface{}
	if strategy.OptionLegJSON != nil {
		optionLeg = string(strategy.OptionLegJSON)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE strategies SET
			name                 = $2,
			description          = $3,
			underlying           = $4,
			instrument_type      = $5,
			expiry_rule          = $6,
			option_leg_json      = $7::jsonb,
			entry_condition_type = $8,
			target_pct           = $9,
			stop_loss_pct        = $10,
			time_exit_minutes    = $11,
			lot_size             = $12,
			capital_per_trade    = $13,
			mode                 = $14,
			is_ready_for_run     = $15,
			updated_at           = NOW()
		WHERE id = $1`,
		strategy.ID, strategy.Name, strategy.Description, strategy.Underlying,
		strategy.InstrumentType, strategy.ExpiryRule, optionLeg,
		strategy.EntryConditionType, strategy.TargetPct, strategy.StopLossPct,
		strategy.TimeExitMinutes, strategy.LotSize, strategy.CapitalPerTrade,
		strategy.Mode, strategy.IsReadyForRun,
	)
	return err
}

func (s *StrategyStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM strategies WHERE id = $1`, id)
	return err
}

func scanStrategy(row pgx.Row) (*entities.Strategy, error) {
	var st entities.Strategy
	var optionLegBytes []byte
	var targetPct, stopLossPct, capitalPerTrade *float64
	var timeExitMinutes *int
	err := row.Scan(
		&st.ID, &st.Name, &st.Description, &st.Underlying, &st.InstrumentType,
		&st.ExpiryRule, &optionLegBytes, &st.EntryConditionType,
		&targetPct, &stopLossPct, &timeExitMinutes,
		&st.LotSize, &capitalPerTrade, &st.Mode, &st.IsReadyForRun,
		&st.CreatedAt, &st.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	st.OptionLegJSON = optionLegBytes
	st.TargetPct = targetPct
	st.StopLossPct = stopLossPct
	st.TimeExitMinutes = timeExitMinutes
	st.CapitalPerTrade = capitalPerTrade
	return &st, nil
}

func scanStrategyRow(rows pgx.Rows) (*entities.Strategy, error) {
	var st entities.Strategy
	var optionLegBytes []byte
	var targetPct, stopLossPct, capitalPerTrade *float64
	var timeExitMinutes *int
	err := rows.Scan(
		&st.ID, &st.Name, &st.Description, &st.Underlying, &st.InstrumentType,
		&st.ExpiryRule, &optionLegBytes, &st.EntryConditionType,
		&targetPct, &stopLossPct, &timeExitMinutes,
		&st.LotSize, &capitalPerTrade, &st.Mode, &st.IsReadyForRun,
		&st.CreatedAt, &st.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	st.OptionLegJSON = optionLegBytes
	st.TargetPct = targetPct
	st.StopLossPct = stopLossPct
	st.TimeExitMinutes = timeExitMinutes
	st.CapitalPerTrade = capitalPerTrade
	return &st, nil
}
