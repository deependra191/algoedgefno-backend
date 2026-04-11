package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Strategy struct {
	ID                 uuid.UUID        `json:"id"`
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	Underlying         string           `json:"underlying"`
	InstrumentType     string           `json:"instrument_type"`
	ExpiryRule         string           `json:"expiry_rule"`
	OptionLeg          *json.RawMessage `json:"option_leg,omitempty"`
	EntryConditionType string           `json:"entry_condition_type"`
	TargetPct          *float64         `json:"target_pct,omitempty"`
	StopLossPct        *float64         `json:"stop_loss_pct,omitempty"`
	TimeExitMinutes    *int             `json:"time_exit_minutes,omitempty"`
	LotSize            int              `json:"lot_size"`
	CapitalPerTrade    *float64         `json:"capital_per_trade,omitempty"`
	Mode               string           `json:"mode"`
	IsReadyForRun      bool             `json:"is_ready_for_run"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}
