package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StrategyRepository is the storage contract for user-defined trading strategies.
type StrategyRepository interface {
	// GetByID returns the strategy with the given ID, or models.ErrNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*Strategy, error)
	// List returns all strategies ordered by last-updated descending.
	List(ctx context.Context) ([]Strategy, error)
	// Create persists a new strategy, assigning a UUID if the ID is zero.
	Create(ctx context.Context, strategy *Strategy) error
	// Update overwrites all mutable fields of an existing strategy.
	Update(ctx context.Context, strategy *Strategy) error
	// Delete removes the strategy with the given ID.
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
	LotSize int
	Mode    string
	IsReadyForRun      bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

const (
	EntryConditionMACrossover   = "MA_CROSSOVER"
	EntryConditionSupertrend    = "SUPERTREND"
	EntryConditionRSIOversold   = "RSI_OVERSOLD"
	EntryConditionMomentum      = "MOMENTUM"
	EntryConditionVWAPCrossover = "VWAP_CROSSOVER"
)

// ExpiryRule constants for when a futures contract rolls.
const (
	ExpiryRuleCurrentMonth = "CURRENT_MONTH"
)
