package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

// Strategy is the domain representation of a user-defined trading strategy.
// OptionLeg is left as an opaque raw JSON message; typed parsing is deferred
// to the engine evaluator (B09).
type Strategy struct {
	ID                 uuid.UUID
	Name               string
	Description        string
	Underlying         string
	InstrumentType     string
	ExpiryRule         string
	OptionLeg          *json.RawMessage
	EntryConditionType string
	TargetPct          *float64
	StopLossPct        *float64
	TimeExitMinutes    *int
	LotSize            int
	CapitalPerTrade    *float64
	Mode               string
	IsReadyForRun      bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func FromStrategyEntity(e *entities.Strategy) *Strategy {
	if e == nil {
		return nil
	}
	s := &Strategy{
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
	}
	if e.OptionLegJSON != nil {
		raw := json.RawMessage(e.OptionLegJSON)
		s.OptionLeg = &raw
	}
	return s
}

func (s *Strategy) ToEntity() *entities.Strategy {
	if s == nil {
		return nil
	}
	e := &entities.Strategy{
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
	}
	if s.OptionLeg != nil {
		e.OptionLegJSON = []byte(*s.OptionLeg)
	}
	return e
}
