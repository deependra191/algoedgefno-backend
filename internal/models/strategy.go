package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type StrategyRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Strategy, error)
	List(ctx context.Context) ([]Strategy, error)
	Create(ctx context.Context, strategy *Strategy) error
	Update(ctx context.Context, strategy *Strategy) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// Strategy is the domain representation of a user-defined trading strategy.
type Strategy struct {
	ID                 uuid.UUID
	Name               string
	Description        string
	Underlying         string
	InstrumentType     string
	ExpiryRule         string
	OptionLeg          json.RawMessage
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

const (
	EntryConditionMACrossover = "MA_CROSSOVER"
	EntryConditionSupertrend  = "SUPERTREND"
	EntryConditionRSIOversold = "RSI_OVERSOLD"
	EntryConditionMomentum    = "MOMENTUM"
)
