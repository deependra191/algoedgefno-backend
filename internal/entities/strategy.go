package entities

import (
	"time"

	"github.com/google/uuid"
)

// Strategy is the DB row for the strategies table.
// OptionLegJSON holds the raw jsonb column bytes; typing is deferred to B09.
type Strategy struct {
	ID                 uuid.UUID
	Name               string
	Description        string
	Underlying         string
	InstrumentType     string
	ExpiryRule         string
	OptionLegJSON      []byte
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
