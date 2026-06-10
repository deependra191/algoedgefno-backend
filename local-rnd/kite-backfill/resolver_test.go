package main

import (
	"testing"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

func TestResolveTargets_SpotIndexAndEquity(t *testing.T) {
	asOf := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	resolver := newInstrumentResolver([]kite.Instrument{
		{
			InstrumentToken: "256265",
			TradingSymbol:   "NIFTY 50",
			Name:            "NIFTY 50",
			Exchange:        kite.ExchangeNSE,
			InstrumentType:  kite.InstrumentTypeIndex,
		},
		{
			InstrumentToken: "738561",
			TradingSymbol:   "RELIANCE",
			Name:            "RELIANCE",
			Exchange:        kite.ExchangeNSE,
			InstrumentType:  kite.InstrumentTypeEquity,
		},
	}, asOf)

	targets, err := resolveTargets(resolver, options{
		symbols:         []string{"NIFTY 50", "RELIANCE"},
		symbolsExplicit: true,
	})
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	index := targets[0].ModelInstrument
	if index.Symbol != models.UnderlyingNifty || index.InstrumentType != models.InstrumentTypeIndex {
		t.Fatalf("index mapped to symbol=%s type=%s", index.Symbol, index.InstrumentType)
	}
	if index.Underlying == nil || *index.Underlying != models.UnderlyingNifty {
		t.Fatalf("index underlying = %v", index.Underlying)
	}

	equity := targets[1].ModelInstrument
	if equity.Symbol != "RELIANCE" || equity.InstrumentType != models.InstrumentTypeEquity {
		t.Fatalf("equity mapped to symbol=%s type=%s", equity.Symbol, equity.InstrumentType)
	}
	if equity.Underlying == nil || *equity.Underlying != "RELIANCE" {
		t.Fatalf("equity underlying = %v", equity.Underlying)
	}
}

func TestResolveTargets_CurrentNFOFuture(t *testing.T) {
	asOf := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	resolver := newInstrumentResolver([]kite.Instrument{
		{
			InstrumentToken: "123",
			TradingSymbol:   "NIFTY26JUNFUT",
			Name:            models.UnderlyingNifty,
			Expiry:          "2026-06-25",
			LotSize:         "75",
			InstrumentType:  kite.InstrumentTypeFuture,
			Segment:         kite.SegmentNFOFutures,
			Exchange:        kite.ExchangeNFO,
		},
	}, asOf)

	targets, err := resolveTargets(resolver, options{
		currentFNO: []string{"NIFTY26JUNFUT"},
	})
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	inst := targets[0].ModelInstrument
	if inst.Symbol != "NIFTY26JUNFUT" || inst.Exchange != models.ExchangeNFO {
		t.Fatalf("future mapped to %s:%s", inst.Exchange, inst.Symbol)
	}
	if inst.InstrumentType != models.InstrumentTypeFuturesIndex {
		t.Fatalf("future type = %s", inst.InstrumentType)
	}
	if inst.Underlying == nil || *inst.Underlying != models.UnderlyingNifty {
		t.Fatalf("future underlying = %v", inst.Underlying)
	}
	if inst.Expiry == nil || inst.Expiry.Format(dateLayout) != "2026-06-25" {
		t.Fatalf("future expiry = %v", inst.Expiry)
	}
	if inst.LotSize != 75 {
		t.Fatalf("future lot size = %d", inst.LotSize)
	}
}
