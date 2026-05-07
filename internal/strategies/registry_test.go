package strategies

import (
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func TestNewRegistry_ContainsMACrossover(t *testing.T) {
	r := NewRegistry()
	s, ok := r.Get(slugMACrossover)
	if !ok {
		t.Fatal("expected ma_crossover to be registered")
	}
	if s.ID != slugMACrossover {
		t.Errorf("expected ID %q, got %q", slugMACrossover, s.ID)
	}
}

func TestGet_Nonexistent(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected false for nonexistent slug")
	}
}

func TestAll_ReturnsInOrder(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	if len(all) == 0 {
		t.Fatal("expected at least one strategy")
	}
	if all[0].ID != slugMACrossover {
		t.Errorf("expected first strategy to be %q, got %q", slugMACrossover, all[0].ID)
	}
}

func TestMACrossover_Fields(t *testing.T) {
	s := MACrossover()
	if s.Name == "" {
		t.Error("expected non-empty Name")
	}
	if s.Category == "" {
		t.Error("expected non-empty Category")
	}
	if s.EntryConditionType != models.EntryConditionMACrossover {
		t.Errorf("expected EntryConditionType %q, got %q", models.EntryConditionMACrossover, s.EntryConditionType)
	}
	if s.InstrumentType != models.InstrumentTypeFuturesIndex {
		t.Errorf("expected InstrumentType %q, got %q", models.InstrumentTypeFuturesIndex, s.InstrumentType)
	}
	if len(s.Inputs) == 0 {
		t.Error("expected at least one input")
	}
	if len(s.Logic) == 0 {
		t.Error("expected at least one logic line")
	}
	sources := s.SourceResolver.Sources(models.StrategyParams{Underlying: models.UnderlyingNifty})
	if sources.Signal.Kind != models.InstrumentKindIndex {
		t.Errorf("expected signal kind %q, got %q", models.InstrumentKindIndex, sources.Signal.Kind)
	}
	if sources.Trade.Kind != models.InstrumentKindFuturesIndexContinuous {
		t.Errorf("expected trade kind %q, got %q", models.InstrumentKindFuturesIndexContinuous, sources.Trade.Kind)
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate slug registration")
		}
	}()
	r := NewRegistry()
	r.register(MACrossover())
}
