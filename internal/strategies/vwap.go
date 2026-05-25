package strategies

import "github.com/deependra191/algoedgefno-backend/internal/models"

const slugVWAPCrossover = "vwap_crossover"

// VWAPCrossover returns the built-in VWAP Crossover strategy definition.
func VWAPCrossover() *models.BuiltinStrategy {
	return &models.BuiltinStrategy{
		ID:          slugVWAPCrossover,
		Name:        "VWAP Crossover",
		Category:    "Momentum",
		Description: "Go long when price closes above the rolling 20-day VWAP. Exit on reverse crossover, stop loss, or session end.",
		Logic: []string{
			"Buy when the daily close crosses above the 20-period rolling VWAP.",
			"Sell when the daily close crosses below the 20-period rolling VWAP.",
			"VWAP is computed as Σ(TypicalPrice × Volume) / Σ(Volume) over the lookback window.",
			"Exit on reverse crossover, stop loss hit, or end of data.",
		},
		Inputs: []models.StrategyInput{
			{
				Key:          models.StrategyInputKeyUnderlying,
				Label:        "Underlying",
				Type:         models.InputTypeSelect,
				Options:      []string{models.UnderlyingNifty, models.UnderlyingBankNifty, models.UnderlyingFinNifty},
				DefaultValue: models.UnderlyingNifty,
			},
			{
				Key:   "dateRange",
				Label: "Date range",
				Type:  models.InputTypeDateRange,
				Constraints: map[string]any{
					models.ConstraintMinDate: historicalDataStart,
				},
				DefaultFrom: defaultBacktestFromDate,
				DefaultTo:   defaultBacktestToDate,
			},
			{
				Key:          "lots",
				Label:        "Lots",
				Type:         models.InputTypeStepper,
				Constraints:  map[string]any{models.ConstraintMin: 1, models.ConstraintMax: 50},
				DefaultValue: 1,
			},
			{
				Key:          "capital",
				Label:        "Capital",
				Type:         models.InputTypeNumberInput,
				Prefix:       "₹",
				Constraints:  map[string]any{models.ConstraintMin: 10000, models.ConstraintMax: 10000000},
				DefaultValue: 200000,
			},
			{
				Key:           "slippagePct",
				Label:         "Slippage · per leg",
				Type:          models.InputTypeNumberInput,
				Suffix:        "%",
				Constraints:   map[string]any{models.ConstraintMin: 0, models.ConstraintMax: 1, models.ConstraintDecimals: 3},
				DefaultValue:  0,
				Caption:       "Applied on entry and exit. Type decimal percent — e.g. 0.005 for 0.005%.",
				CaptionAtZero: "No slippage assumed — backtest uses ideal fills.",
			},
		},
		EntryConditionType: models.EntryConditionVWAPCrossover,
		InstrumentType:     models.InstrumentTypeFuturesIndex,
		ExpiryRule:         models.ExpiryRuleCurrentMonth,
		CandleInterval:     models.CandleInterval1D,
		SourceResolver:     indexSignalFuturesIndexTradeSources(),
	}
}
