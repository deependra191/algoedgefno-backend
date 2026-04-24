package strategies

import "github.com/deependra191/algoedgefno-backend/internal/models"

const (
	slugMACrossover = "ma_crossover"
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
				Type:         "SELECT",
				Options:      []string{"NIFTY", "BANKNIFTY", "FINNIFTY"},
				DefaultValue: "NIFTY",
			},
			{
				Key:   "dateRange",
				Label: "Date range",
				Type:  "DATE_RANGE",
				Constraints: map[string]any{
					"minDate": "2022-01-01",
					// maxDate is filled dynamically by the service at request time
				},
				DefaultFrom: "2025-01-01",
				DefaultTo:   "2025-12-31",
			},
			{
				Key:          "lots",
				Label:        "Lots",
				Type:         "NUMBER",
				Constraints:  map[string]any{"min": 1, "max": 50},
				DefaultValue: 1,
			},
			{
				Key:          "capital",
				Label:        "Capital",
				Type:         "CURRENCY",
				Constraints:  map[string]any{"min": 10000, "max": 10000000},
				DefaultValue: 200000,
			},
		},
		EntryConditionType: models.EntryConditionMACrossover,
		InstrumentType:     "FUTIDX",
		ExpiryRule:         "CURRENT_MONTH",
		CandleInterval:     "1d",
	}
}
