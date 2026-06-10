package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

type fakeHistoricalClient struct {
	requests []kite.HistoricalRequest
	result   *kite.HistoricalResult
	err      error
}

func (f *fakeHistoricalClient) FetchHistorical(_ context.Context, r kite.HistoricalRequest) (*kite.HistoricalResult, error) {
	f.requests = append(f.requests, r)
	return f.result, f.err
}

type fakeCandleStore struct {
	inserted []models.Candle
	existing []models.Candle
	deleted  int64
}

func (f *fakeCandleStore) Query(_ context.Context, _ models.CandleFilter) ([]models.Candle, error) {
	return f.existing, nil
}

func (f *fakeCandleStore) InsertBatchIgnoreDuplicates(_ context.Context, candles []models.Candle) (int64, error) {
	f.inserted = append(f.inserted, candles...)
	return int64(len(candles)), nil
}

func (f *fakeCandleStore) DeleteRange(_ context.Context, _ models.CandleFilter) (int64, error) {
	return f.deleted, nil
}

func TestImporterImportsFixtureKiteCandlesWithoutNetwork(t *testing.T) {
	loc, err := time.LoadLocation(istLocationName)
	if err != nil {
		t.Fatalf("load IST: %v", err)
	}
	instrumentID := uuid.New()
	kiteTimestamp := time.Date(2025, 6, 10, 9, 15, 0, 0, loc)
	client := &fakeHistoricalClient{
		result: &kite.HistoricalResult{
			HTTPStatus: http.StatusOK,
			APIStatus:  kite.APIStatusSuccess,
			Candles: []kite.Candle{
				{Timestamp: kiteTimestamp, Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 10},
				{Timestamp: kiteTimestamp.Add(time.Minute), Open: 100.5, High: 102, Low: 100, Close: 101, Volume: 20},
			},
		},
	}
	store := &fakeCandleStore{}
	runner := importer{
		client:  client,
		candles: store,
		opts: options{
			fromDate:         time.Date(2025, 6, 10, 0, 0, 0, 0, loc),
			toDate:           time.Date(2025, 6, 10, 0, 0, 0, 0, loc),
			delay:            0,
			timeout:          time.Second,
			intradayLocation: loc,
		},
		runID: uuid.New(),
	}
	target := importTarget{
		RequestedSymbol: "RELIANCE",
		KiteInstrument: kite.Instrument{
			InstrumentToken: "738561",
			TradingSymbol:   "RELIANCE",
			Exchange:        kite.ExchangeNSE,
		},
		ModelInstrument: models.Instrument{
			ID:       instrumentID,
			Symbol:   "RELIANCE",
			Exchange: models.ExchangeNSE,
		},
	}

	inserted, err := runner.importTargets(context.Background(), []importTarget{target})
	if err != nil {
		t.Fatalf("import targets: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("inserted = %d, want 2", inserted)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected 1 historical request, got %d", len(client.requests))
	}
	req := client.requests[0]
	if req.Interval != kite.IntervalMinute {
		t.Fatalf("request interval = %s", req.Interval)
	}
	if req.From.Format(dateTimeLayout) != "2025-06-10 09:15:00" || req.To.Format(dateTimeLayout) != "2025-06-10 15:30:00" {
		t.Fatalf("request range = %s to %s", req.From.Format(dateTimeLayout), req.To.Format(dateTimeLayout))
	}
	if len(store.inserted) != 2 {
		t.Fatalf("stored candles = %d, want 2", len(store.inserted))
	}
	first := store.inserted[0]
	if first.InstrumentID != instrumentID {
		t.Fatalf("instrument ID = %s", first.InstrumentID)
	}
	if first.Interval != models.CandleInterval1M {
		t.Fatalf("interval = %s", first.Interval)
	}
	if first.Provider != providerZerodhaKite {
		t.Fatalf("provider = %s", first.Provider)
	}
	if !first.Timestamp.Equal(kiteTimestamp.UTC()) {
		t.Fatalf("timestamp = %s, want %s", first.Timestamp, kiteTimestamp.UTC())
	}
}
