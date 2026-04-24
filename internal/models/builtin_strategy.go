package models

// BuiltinStrategy is a code-defined trading strategy identified by a slug.
type BuiltinStrategy struct {
	ID                 string
	Name               string
	Category           string
	Description        string
	Logic              []string
	Inputs             []StrategyInput
	EntryConditionType string
	InstrumentType     string
	ExpiryRule         string
	CandleInterval     string
	TargetPct          *float64
	StopLossPct        *float64
	TimeExitMinutes    *int
}

// StrategyInput describes a single user-configurable input for a strategy's backtest form.
type StrategyInput struct {
	Key          string
	Label        string
	Type         string
	Options      []string
	Constraints  map[string]any
	DefaultValue any
	DefaultFrom  string
	DefaultTo    string
}

// Input type constants for StrategyInput.Type — used by both the registry and the Android form renderer.
const (
	InputTypeSelect    = "SELECT"
	InputTypeDateRange = "DATE_RANGE"
	InputTypeNumber    = "NUMBER"
	InputTypeCurrency  = "CURRENCY"
)

// Constraint key constants used in StrategyInput.Constraints maps.
const (
	ConstraintMin     = "min"
	ConstraintMax     = "max"
	ConstraintMinDate = "minDate"
)

// BuiltinStrategyLookup is the contract for resolving code-defined strategies by slug.
type BuiltinStrategyLookup interface {
	Get(slug string) (*BuiltinStrategy, bool)
	All() []*BuiltinStrategy
}

// ToEngineStrategy builds a Strategy suitable for the backtest engine
// from this built-in definition and user-supplied inputs.
func (b *BuiltinStrategy) ToEngineStrategy(underlying string, lots int) *Strategy {
	return &Strategy{
		Name:               b.Name,
		Description:        b.Description,
		Underlying:         underlying,
		InstrumentType:     b.InstrumentType,
		ExpiryRule:         b.ExpiryRule,
		EntryConditionType: b.EntryConditionType,
		TargetPct:          b.TargetPct,
		StopLossPct:        b.StopLossPct,
		TimeExitMinutes:    b.TimeExitMinutes,
		LotSize:            lots,
	}
}
