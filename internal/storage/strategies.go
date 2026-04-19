package storage

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.StrategyRepository = (*StrategyStore)(nil)

type StrategyStore struct {
	pool *pgxpool.Pool
}

func NewStrategyStore(pool *pgxpool.Pool) *StrategyStore {
	return &StrategyStore{pool: pool}
}

func (s *StrategyStore) Create(ctx context.Context, strategy *models.Strategy) error {
	if strategy.ID == uuid.Nil {
		strategy.ID = uuid.New()
	}
	ent := toStrategyEntity(strategy)
	var optionLeg any
	if ent.OptionLegJSON != nil {
		optionLeg = string(ent.OptionLegJSON)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO strategies
			(id, name, description, underlying, instrument_type, expiry_rule, option_leg_json,
			 entry_condition_type, target_pct, stop_loss_pct, time_exit_minutes, lot_size,
			 capital_per_trade, mode, is_ready_for_run, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13,$14,$15,NOW(),NOW())`,
		ent.ID, ent.Name, ent.Description, ent.Underlying,
		ent.InstrumentType, ent.ExpiryRule, optionLeg,
		ent.EntryConditionType, ent.TargetPct, ent.StopLossPct,
		ent.TimeExitMinutes, ent.LotSize, ent.CapitalPerTrade,
		ent.Mode, ent.IsReadyForRun,
	)
	return err
}

func (s *StrategyStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Strategy, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, description, underlying, instrument_type, expiry_rule, option_leg_json,
		       entry_condition_type, target_pct, stop_loss_pct, time_exit_minutes, lot_size,
		       capital_per_trade, mode, is_ready_for_run, created_at, updated_at
		FROM strategies WHERE id = $1`, id)
	ent, err := scanStrategy(row)
	if err != nil {
		return nil, err
	}
	return toStrategyModel(ent), nil
}

func (s *StrategyStore) List(ctx context.Context) ([]models.Strategy, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, underlying, instrument_type, expiry_rule, option_leg_json,
		       entry_condition_type, target_pct, stop_loss_pct, time_exit_minutes, lot_size,
		       capital_per_trade, mode, is_ready_for_run, created_at, updated_at
		FROM strategies ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.Strategy
	for rows.Next() {
		ent, err := scanStrategyRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *toStrategyModel(ent))
	}
	return result, rows.Err()
}

func (s *StrategyStore) Update(ctx context.Context, strategy *models.Strategy) error {
	ent := toStrategyEntity(strategy)
	var optionLeg any
	if ent.OptionLegJSON != nil {
		optionLeg = string(ent.OptionLegJSON)
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
		ent.ID, ent.Name, ent.Description, ent.Underlying,
		ent.InstrumentType, ent.ExpiryRule, optionLeg,
		ent.EntryConditionType, ent.TargetPct, ent.StopLossPct,
		ent.TimeExitMinutes, ent.LotSize, ent.CapitalPerTrade,
		ent.Mode, ent.IsReadyForRun,
	)
	return err
}

func (s *StrategyStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM strategies WHERE id = $1`, id)
	return err
}

func toStrategyModel(e *entities.Strategy) *models.Strategy {
	s := &models.Strategy{
		ID:                 e.ID,
		Name:               e.Name,
		Description:        e.Description,
		Underlying:         e.Underlying,
		InstrumentType:     e.InstrumentType,
		ExpiryRule:         e.ExpiryRule,
		EntryConditionType: e.EntryConditionType,
		TargetPct:          e.TargetPct,
		StopLossPct:        e.StopLossPct,
		TimeExitMinutes:    e.TimeExitMinutes,
		LotSize:            e.LotSize,
		CapitalPerTrade:    e.CapitalPerTrade,
		Mode:               e.Mode,
		IsReadyForRun:      e.IsReadyForRun,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
		OptionLeg:          json.RawMessage(e.OptionLegJSON),
	}
	return s
}

func toStrategyEntity(s *models.Strategy) *entities.Strategy {
	return &entities.Strategy{
		ID:                 s.ID,
		Name:               s.Name,
		Description:        s.Description,
		Underlying:         s.Underlying,
		InstrumentType:     s.InstrumentType,
		ExpiryRule:         s.ExpiryRule,
		EntryConditionType: s.EntryConditionType,
		TargetPct:          s.TargetPct,
		StopLossPct:        s.StopLossPct,
		TimeExitMinutes:    s.TimeExitMinutes,
		LotSize:            s.LotSize,
		CapitalPerTrade:    s.CapitalPerTrade,
		Mode:               s.Mode,
		IsReadyForRun:      s.IsReadyForRun,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
		OptionLegJSON:      []byte(s.OptionLeg),
	}
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
