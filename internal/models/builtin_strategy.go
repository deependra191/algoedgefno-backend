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
	SourceResolver     StrategySourceResolver
}

// StrategyInput describes a single user-configurable input for a strategy's backtest form.
// Prefix, Suffix, Caption, and CaptionAtZero are used by NUMBER_INPUT to provide
// contextual hints in the Android form renderer.
type StrategyInput struct {
	Key           string
	Label         string
	Type          string
	Options       []string
	Constraints   map[string]any
	DefaultValue  any
	DefaultFrom   string
	DefaultTo     string
	Prefix        string
	Suffix        string
	Caption       string
	CaptionAtZero string
}

// Input type constants for StrategyInput.Type — used by both the registry and the Android form renderer.
const (
	InputTypeSelect      = "SELECT"
	InputTypeDateRange   = "DATE_RANGE"
	InputTypeStepper     = "STEPPER"
	InputTypeNumberInput = "NUMBER_INPUT"
)

// Strategy input key constants used by built-ins and services.
const (
	StrategyInputKeyUnderlying = "underlying"
)

// Constraint key constants used in StrategyInput.Constraints maps.
const (
	ConstraintMin      = "min"
	ConstraintMax      = "max"
	ConstraintMinDate  = "minDate"
	ConstraintDecimals = "decimals"
)

// BuiltinStrategyLookup is the contract for resolving code-defined strategies by slug.
type BuiltinStrategyLookup interface {
	Get(slug string) (*BuiltinStrategy, bool)
	All() []*BuiltinStrategy
}
