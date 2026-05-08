package models

import "testing"

const unsupportedStrategySourceInterval = "1m"

func TestValidateStrategySourceIntervals_CurrentlySupportsDailyForAllKinds(t *testing.T) {
	tests := []struct {
		name string
		kind InstrumentKind
	}{
		{name: "index", kind: InstrumentKindIndex},
		{name: "equity", kind: InstrumentKindEquity},
		{name: "continuous futures index", kind: InstrumentKindFuturesIndexContinuous},
		{name: "continuous futures stock", kind: InstrumentKindFuturesStockContinuous},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := StrategySources{
				Signal: InstrumentSpec{Kind: tt.kind},
				Trade:  InstrumentSpec{Kind: tt.kind},
			}
			if err := ValidateStrategySourceIntervals(sources, CandleInterval1D); err != nil {
				t.Fatalf("expected daily interval to be supported: %v", err)
			}
			if err := ValidateStrategySourceIntervals(sources, unsupportedStrategySourceInterval); err == nil {
				t.Fatal("expected unsupported interval to be rejected")
			}
		})
	}
}
