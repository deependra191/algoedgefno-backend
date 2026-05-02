package strategies

import "github.com/deependra191/algoedgefno-backend/internal/models"

const (
	slugMACrossover    = "ma_crossover"
	historicalDataStart = "2022-01-01"
)

// MACrossover returns the built-in MA Crossover strategy definition.
func MACrossover() *models.BuiltinStrategy {
	return &models.BuiltinStrategy{
		ID:          slugMACrossover,
		Name:        "MA Crossover",
		Category:    "Trend",
		Description: "Go long when the short-period SMA crosses above the long-period SMA. Exit on reverse crossover, stop loss, or session end.",
		Logic: []string{
			"Buy when the 9-period SMA crosses above the 21-period SMA.",
			"Sell when the 9-period SMA crosses below the 21-period SMA.",
			"Timeframe is fixed at daily candles.",
			"Exit on reverse crossover, stop loss hit, or end of data.",
		},
		Inputs: []models.StrategyInput{
			{
				Key:          "underlying",
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
					// maxDate is filled dynamically by the service at request time
				},
				DefaultFrom: "2025-01-01",
				DefaultTo:   "2025-12-31",
			},
			{
				Key:          "lots",
				Label:        "Lots",
				Type:         models.InputTypeNumber,
				Constraints:  map[string]any{models.ConstraintMin: 1, models.ConstraintMax: 50},
				DefaultValue: 1,
			},
			{
				Key:          "capital",
				Label:        "Capital",
				Type:         models.InputTypeCurrency,
				Constraints:  map[string]any{models.ConstraintMin: 10000, models.ConstraintMax: 10000000},
				DefaultValue: 200000,
			},
		},
		EntryConditionType: models.EntryConditionMACrossover,
		InstrumentType:     models.InstrumentTypeFuturesIndex,
		ExpiryRule:         models.ExpiryRuleCurrentMonth,
		CandleInterval:     models.CandleInterval1D,
	}
}
