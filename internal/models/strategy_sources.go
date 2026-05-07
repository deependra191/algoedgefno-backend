package models

import "fmt"

// InstrumentKind identifies the class of instrument a strategy wants for a candle stream.
type InstrumentKind string

const (
	InstrumentKindIndex                  InstrumentKind = InstrumentKind(InstrumentTypeIndex)
	InstrumentKindEquity                 InstrumentKind = InstrumentKind(InstrumentTypeEquity)
	InstrumentKindFuturesIndex           InstrumentKind = InstrumentKind(InstrumentTypeFuturesIndex)
	InstrumentKindFuturesStock           InstrumentKind = InstrumentKind(InstrumentTypeFuturesStock)
	InstrumentKindFuturesIndexContinuous InstrumentKind = InstrumentKind(InstrumentTypeFuturesIndexCont)
	InstrumentKindFuturesStockContinuous InstrumentKind = InstrumentKind(InstrumentTypeFuturesStockCont)
	InstrumentKindOptionsIndex           InstrumentKind = InstrumentKind(InstrumentTypeOptionsIndex)
	InstrumentKindOptionsStock           InstrumentKind = InstrumentKind(InstrumentTypeOptionsStock)
)

// StrategyParams carries runtime strategy inputs that may affect source resolution.
type StrategyParams struct {
	Underlying string
}

// InstrumentSpec declares the instrument class used by one side of a strategy.
type InstrumentSpec struct {
	Kind InstrumentKind
}

// StrategySources declares independent signal-side and trade-side candle sources.
type StrategySources struct {
	Signal InstrumentSpec
	Trade  InstrumentSpec
}

// StrategySourceResolver resolves strategy parameters to signal and trade sources.
type StrategySourceResolver interface {
	Sources(params StrategyParams) StrategySources
}

// ValidateStrategySourceIntervals checks that one strategy interval is supported by both source kinds.
func ValidateStrategySourceIntervals(sources StrategySources, interval string) error {
	if !supportsStrategyInterval(sources.Signal.Kind, interval) {
		return fmt.Errorf("signal source kind %s does not support interval %s", sources.Signal.Kind, interval)
	}
	if !supportsStrategyInterval(sources.Trade.Kind, interval) {
		return fmt.Errorf("trade source kind %s does not support interval %s", sources.Trade.Kind, interval)
	}
	return nil
}

func supportsStrategyInterval(kind InstrumentKind, interval string) bool {
	switch kind {
	case InstrumentKindIndex,
		InstrumentKindEquity,
		InstrumentKindFuturesIndex,
		InstrumentKindFuturesStock,
		InstrumentKindFuturesIndexContinuous,
		InstrumentKindFuturesStockContinuous,
		InstrumentKindOptionsIndex,
		InstrumentKindOptionsStock:
		return interval == CandleInterval1D
	default:
		return false
	}
}
