package storage

import (
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func toStrategyModel(e *entities.Strategy) *models.Strategy {
	return &models.Strategy{
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
		Mode:               e.Mode,
		IsReadyForRun:      e.IsReadyForRun,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
		OptionLeg:          json.RawMessage(e.OptionLegJSON),
	}
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
	var targetPct, stopLossPct *float64
	var timeExitMinutes *int
	err := row.Scan(
		&st.ID, &st.Name, &st.Description, &st.Underlying, &st.InstrumentType,
		&st.ExpiryRule, &optionLegBytes, &st.EntryConditionType,
		&targetPct, &stopLossPct, &timeExitMinutes,
		&st.LotSize, &st.Mode, &st.IsReadyForRun,
		&st.CreatedAt, &st.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	st.OptionLegJSON = optionLegBytes
	st.TargetPct = targetPct
	st.StopLossPct = stopLossPct
	st.TimeExitMinutes = timeExitMinutes
	return &st, nil
}

func scanStrategyRow(rows pgx.Rows) (*entities.Strategy, error) {
	var st entities.Strategy
	var optionLegBytes []byte
	var targetPct, stopLossPct *float64
	var timeExitMinutes *int
	err := rows.Scan(
		&st.ID, &st.Name, &st.Description, &st.Underlying, &st.InstrumentType,
		&st.ExpiryRule, &optionLegBytes, &st.EntryConditionType,
		&targetPct, &stopLossPct, &timeExitMinutes,
		&st.LotSize, &st.Mode, &st.IsReadyForRun,
		&st.CreatedAt, &st.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	st.OptionLegJSON = optionLegBytes
	st.TargetPct = targetPct
	st.StopLossPct = stopLossPct
	st.TimeExitMinutes = timeExitMinutes
	return &st, nil
}
