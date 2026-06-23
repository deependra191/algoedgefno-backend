package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

type importTarget struct {
	RequestedSymbol string
	KiteInstrument  kite.Instrument
	ModelInstrument models.Instrument
}

type targetKey struct {
	Symbol   string
	Exchange string
}

type instrumentResolver struct {
	byExchangeSymbol map[targetKey]kite.Instrument
	asOf             time.Time
}

var kiteIndexSymbolToUnderlying = map[string]string{
	"NIFTY 50":   models.UnderlyingNifty,
	"NIFTY BANK": models.UnderlyingBankNifty,
}

func newInstrumentResolver(instruments []kite.Instrument, asOf time.Time) instrumentResolver {
	byExchangeSymbol := make(map[targetKey]kite.Instrument, len(instruments))
	for _, inst := range instruments {
		key := targetKey{
			Symbol:   strings.ToUpper(strings.TrimSpace(inst.TradingSymbol)),
			Exchange: strings.ToUpper(strings.TrimSpace(inst.Exchange)),
		}
		if key.Symbol == "" || key.Exchange == "" {
			continue
		}
		byExchangeSymbol[key] = inst
	}
	return instrumentResolver{byExchangeSymbol: byExchangeSymbol, asOf: asOf}
}

func (r instrumentResolver) resolveSpot(symbol string) (importTarget, bool, error) {
	symbol = strings.TrimSpace(symbol)
	inst, ok := r.byExchangeSymbol[targetKey{
		Symbol:   strings.ToUpper(symbol),
		Exchange: kite.ExchangeNSE,
	}]
	if !ok {
		return importTarget{}, false, nil
	}

	if underlying, isIndex := kiteIndexSymbolToUnderlying[strings.ToUpper(symbol)]; isIndex {
		return importTarget{
			RequestedSymbol: symbol,
			KiteInstrument:  inst,
			ModelInstrument: models.Instrument{
				Symbol:         underlying,
				Name:           symbol,
				Exchange:       models.ExchangeNSE,
				InstrumentType: models.InstrumentTypeIndex,
				Underlying:     &underlying,
				LotSize:        1,
			},
		}, true, nil
	}

	if !strings.EqualFold(inst.Exchange, kite.ExchangeNSE) {
		return importTarget{}, false, fmt.Errorf("%s did not resolve to NSE", symbol)
	}
	if inst.InstrumentType != "" && !strings.EqualFold(inst.InstrumentType, kite.InstrumentTypeEquity) {
		return importTarget{}, false, fmt.Errorf("%s did not resolve to a Kite NSE equity", symbol)
	}
	underlying := strings.ToUpper(symbol)
	return importTarget{
		RequestedSymbol: symbol,
		KiteInstrument:  inst,
		ModelInstrument: models.Instrument{
			Symbol:         underlying,
			Name:           fallbackName(inst.Name, underlying),
			Exchange:       models.ExchangeNSE,
			InstrumentType: models.InstrumentTypeEquity,
			Underlying:     &underlying,
			LotSize:        1,
		},
	}, true, nil
}

func (r instrumentResolver) resolveCurrentFNO(symbol string) (importTarget, bool, error) {
	symbol = strings.TrimSpace(symbol)
	inst, ok := r.byExchangeSymbol[targetKey{
		Symbol:   strings.ToUpper(symbol),
		Exchange: kite.ExchangeNFO,
	}]
	if !ok {
		return importTarget{}, false, nil
	}
	if !strings.EqualFold(inst.InstrumentType, kite.InstrumentTypeFuture) {
		return importTarget{}, false, fmt.Errorf("%s is not a Kite futures contract", symbol)
	}
	if inst.Segment != "" && !strings.EqualFold(inst.Segment, kite.SegmentNFOFutures) {
		return importTarget{}, false, fmt.Errorf("%s is not in Kite NFO futures segment", symbol)
	}

	expiry, ok := kite.ParseKiteDate(inst.Expiry)
	if !ok {
		return importTarget{}, false, fmt.Errorf("%s has invalid Kite expiry %q", symbol, inst.Expiry)
	}
	if expiry.Before(startOfDay(r.asOf, expiry.Location())) {
		return importTarget{}, false, fmt.Errorf("%s expired before this run", symbol)
	}

	underlying := strings.ToUpper(strings.TrimSpace(inst.Name))
	if underlying == "" {
		underlying = inferUnderlyingFromFutureSymbol(symbol)
	}
	instrumentType := models.InstrumentTypeFuturesStock
	if isIndexUnderlying(underlying) {
		instrumentType = models.InstrumentTypeFuturesIndex
	}
	lotSize := parsePositiveInt(inst.LotSize, 1)

	return importTarget{
		RequestedSymbol: symbol,
		KiteInstrument:  inst,
		ModelInstrument: models.Instrument{
			Symbol:         strings.ToUpper(symbol),
			Name:           strings.ToUpper(symbol),
			Exchange:       models.ExchangeNFO,
			InstrumentType: instrumentType,
			Underlying:     &underlying,
			Expiry:         &expiry,
			LotSize:        lotSize,
		},
	}, true, nil
}

func (r instrumentResolver) resolveCurrentOptions(symbol string) (importTarget, bool, error) {
	symbol = strings.TrimSpace(symbol)
	inst, ok := r.byExchangeSymbol[targetKey{
		Symbol:   strings.ToUpper(symbol),
		Exchange: kite.ExchangeNFO,
	}]
	if !ok {
		return importTarget{}, false, nil
	}
	instType := strings.ToUpper(inst.InstrumentType)
	if instType != kite.InstrumentTypeCall && instType != kite.InstrumentTypePut {
		return importTarget{}, false, fmt.Errorf("%s is not a Kite options contract (CE/PE), got %q", symbol, inst.InstrumentType)
	}
	if inst.Segment != "" && !strings.EqualFold(inst.Segment, kite.SegmentNFOOptions) {
		return importTarget{}, false, fmt.Errorf("%s is not in Kite NFO options segment", symbol)
	}

	expiry, ok := kite.ParseKiteDate(inst.Expiry)
	if !ok {
		return importTarget{}, false, fmt.Errorf("%s has invalid Kite expiry %q", symbol, inst.Expiry)
	}
	if expiry.Before(startOfDay(r.asOf, expiry.Location())) {
		return importTarget{}, false, fmt.Errorf("%s expired before this run", symbol)
	}

	strike, err := parseStrike(inst.Strike)
	if err != nil {
		return importTarget{}, false, fmt.Errorf("%s has invalid strike %q: %w", symbol, inst.Strike, err)
	}

	underlying := strings.ToUpper(strings.TrimSpace(inst.Name))
	if underlying == "" {
		underlying = inferUnderlyingFromFutureSymbol(symbol)
	}
	instrumentType := models.InstrumentTypeOptionsStock
	if isIndexUnderlying(underlying) {
		instrumentType = models.InstrumentTypeOptionsIndex
	}
	optionType := instType
	lotSize := parsePositiveInt(inst.LotSize, 1)

	return importTarget{
		RequestedSymbol: symbol,
		KiteInstrument:  inst,
		ModelInstrument: models.Instrument{
			Symbol:         strings.ToUpper(symbol),
			Name:           strings.ToUpper(symbol),
			Exchange:       models.ExchangeNFO,
			InstrumentType: instrumentType,
			Underlying:     &underlying,
			Expiry:         &expiry,
			Strike:         &strike,
			OptionType:     &optionType,
			LotSize:        lotSize,
		},
	}, true, nil
}

func resolveTargets(resolver instrumentResolver, opts options) ([]importTarget, error) {
	targets := make([]importTarget, 0, len(opts.symbols)+len(opts.currentFNO)+len(opts.currentOptions))
	for _, symbol := range opts.symbols {
		target, ok, err := resolver.resolveSpot(symbol)
		if err != nil {
			return nil, err
		}
		if !ok {
			if opts.symbolsExplicit {
				return nil, fmt.Errorf("could not resolve NSE symbol %s in Kite instruments", symbol)
			}
			continue
		}
		targets = append(targets, target)
	}
	for _, symbol := range opts.currentFNO {
		target, ok, err := resolver.resolveCurrentFNO(symbol)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("could not resolve current NFO futures symbol %s in Kite instruments", symbol)
		}
		targets = append(targets, target)
	}
	for _, symbol := range opts.currentOptions {
		target, ok, err := resolver.resolveCurrentOptions(symbol)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("could not resolve current NFO options symbol %s in Kite instruments", symbol)
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no import targets resolved")
	}
	return targets, nil
}

func attachPersistedInstruments(targets []importTarget, instruments []models.Instrument) ([]importTarget, error) {
	byKey := make(map[targetKey]models.Instrument, len(instruments))
	for _, inst := range instruments {
		byKey[targetKey{Symbol: inst.Symbol, Exchange: inst.Exchange}] = inst
	}
	for i := range targets {
		modelInst := targets[i].ModelInstrument
		persisted, ok := byKey[targetKey{Symbol: modelInst.Symbol, Exchange: modelInst.Exchange}]
		if !ok {
			return nil, fmt.Errorf("persisted instrument not found for %s:%s", modelInst.Exchange, modelInst.Symbol)
		}
		targets[i].ModelInstrument = persisted
	}
	return targets, nil
}

func fallbackName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return name
}

func isIndexUnderlying(underlying string) bool {
	switch strings.ToUpper(strings.TrimSpace(underlying)) {
	case models.UnderlyingNifty, models.UnderlyingBankNifty, models.UnderlyingFinNifty:
		return true
	default:
		return false
	}
}

func inferUnderlyingFromFutureSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	for _, underlying := range []string{models.UnderlyingBankNifty, models.UnderlyingFinNifty, models.UnderlyingNifty} {
		if strings.HasPrefix(symbol, underlying) {
			return underlying
		}
	}
	if strings.HasSuffix(symbol, kite.InstrumentTypeFuture) {
		return strings.TrimSuffix(symbol, kite.InstrumentTypeFuture)
	}
	return symbol
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	converted := t.In(loc)
	return time.Date(converted.Year(), converted.Month(), converted.Day(), 0, 0, 0, 0, loc)
}

func parseStrike(value string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}
