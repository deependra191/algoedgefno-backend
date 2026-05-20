package storage

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

type scanRow []any

func (r scanRow) Scan(dest ...any) error {
	if len(dest) != len(r) {
		return fmt.Errorf("scan destination count %d, want %d", len(dest), len(r))
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(r[i]))
	}
	return nil
}

func TestBacktestMapperPreservesResultSlippage(t *testing.T) {
	slippage := 12.34
	model := &models.BacktestRun{
		ID:           uuid.New(),
		Status:       models.BacktestCompleted,
		NetPnl:       ptrFloat(900),
		GrossPnl:     ptrFloat(1000),
		TotalCharges: ptrFloat(87.66),
		Slippage:     &slippage,
		TotalTrades:  ptrInt(4),
	}

	entity := toBacktestEntity(model)
	if entity.Slippage == nil || *entity.Slippage != slippage {
		t.Fatalf("entity slippage = %v, want %.2f", entity.Slippage, slippage)
	}

	roundTrip := toBacktestModel(entity)
	if roundTrip.Slippage == nil || *roundTrip.Slippage != slippage {
		t.Fatalf("model slippage = %v, want %.2f", roundTrip.Slippage, slippage)
	}
}

func TestScanBacktestRunScansResultSlippage(t *testing.T) {
	id := uuid.New()
	strategyID := uuid.New()
	signalToken := "NIFTY"
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	netPnl := 900.0
	totalTrades := 4
	winCount := 3
	lossCount := 1
	maxDrawdown := 0.02
	errMsg := ""
	createdAt := from
	completedAt := to
	strategySlug := "ma_crossover"
	capital := 100000.0
	lots := 2
	underlying := "NIFTY"
	grossPnl := 1000.0
	totalCharges := 87.66
	slippage := 12.34

	row := scanRow{
		id, &strategyID, "NIFTY_FUT_CONT", &signalToken, from, to,
		models.CandleInterval1D, models.BacktestCompleted,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		&errMsg, createdAt, &completedAt,
		&strategySlug, &capital, &lots, &underlying,
		[]byte(`{"LongestWinStreak":2}`), []byte(`{"Equity":[]}`),
		&grossPnl, &totalCharges, &slippage,
	}

	entity, err := scanBacktestRun(row)
	if err != nil {
		t.Fatalf("scanBacktestRun: %v", err)
	}
	if entity.Slippage == nil || *entity.Slippage != slippage {
		t.Fatalf("scanned slippage = %v, want %.2f", entity.Slippage, slippage)
	}
}

func TestScanBacktestRunWithTradesScansResultSlippage(t *testing.T) {
	id := uuid.New()
	strategyID := uuid.New()
	signalToken := "NIFTY"
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	netPnl := 900.0
	totalTrades := 4
	winCount := 3
	lossCount := 1
	maxDrawdown := 0.02
	errMsg := ""
	createdAt := from
	completedAt := to
	strategySlug := "ma_crossover"
	capital := 100000.0
	lots := 2
	underlying := "NIFTY"
	grossPnl := 1000.0
	totalCharges := 87.66
	slippage := 12.34

	row := scanRow{
		id, &strategyID, "NIFTY_FUT_CONT", &signalToken, from, to,
		models.CandleInterval1D, models.BacktestCompleted,
		&netPnl, &totalTrades, &winCount, &lossCount, &maxDrawdown,
		[]byte(`[{"Slippage":12.34}]`), &errMsg, createdAt, &completedAt,
		&strategySlug, &capital, &lots, &underlying,
		&grossPnl, &totalCharges, &slippage,
	}

	entity, err := scanBacktestRunWithTrades(row)
	if err != nil {
		t.Fatalf("scanBacktestRunWithTrades: %v", err)
	}
	if entity.Slippage == nil || *entity.Slippage != slippage {
		t.Fatalf("scanned slippage = %v, want %.2f", entity.Slippage, slippage)
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}

func ptrInt(v int) *int {
	return &v
}
