package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

type historicalClient interface {
	FetchHistorical(ctx context.Context, r kite.HistoricalRequest) (*kite.HistoricalResult, error)
}

type candleStore interface {
	Query(ctx context.Context, f models.CandleFilter) ([]models.Candle, error)
	InsertBatchIgnoreDuplicates(ctx context.Context, candles []models.Candle) (int64, error)
	DeleteRange(ctx context.Context, filter models.CandleFilter) (int64, error)
}

type importer struct {
	client  historicalClient
	candles candleStore
	opts    options
	runID   uuid.UUID
}

func (i importer) importTargets(ctx context.Context, targets []importTarget) (int64, error) {
	var totalInserted int64
	for _, target := range targets {
		inserted, err := i.importTarget(ctx, target)
		if err != nil {
			return totalInserted, err
		}
		totalInserted += inserted
	}
	return totalInserted, nil
}

func (i importer) importTarget(ctx context.Context, target importTarget) (int64, error) {
	if i.opts.replace {
		return i.replaceTarget(ctx, target)
	}
	return i.fetchAndInsertTarget(ctx, target)
}

func (i importer) replaceTarget(ctx context.Context, target importTarget) (int64, error) {
	filter := i.candleFilter(target)
	queryCtx, queryCancel := context.WithTimeout(ctx, i.opts.timeout)
	existing, err := i.candles.Query(queryCtx, filter)
	queryCancel()
	if err != nil {
		return 0, fmt.Errorf("query existing candles for %s: %w", target.RequestedSymbol, err)
	}
	runDir := replacementRunDir(i.opts.replacementRoot, i.runID)
	backupPath, err := writeCandleBackup(runDir, target, existing, i.opts.fromDate, i.opts.toDate)
	if err != nil {
		return 0, err
	}
	deleteCtx, deleteCancel := context.WithTimeout(ctx, i.opts.timeout)
	deleted, err := i.candles.DeleteRange(deleteCtx, filter)
	deleteCancel()
	if err != nil {
		return 0, fmt.Errorf("delete existing candles for %s: %w", target.RequestedSymbol, err)
	}
	inserted, err := i.fetchAndInsertTarget(ctx, target)
	if err != nil {
		return inserted, err
	}
	if err := appendReplacementAudit(runDir, replacementAudit{
		RunID:        i.runID,
		Target:       target,
		From:         i.opts.fromDate,
		To:           i.opts.toDate,
		Reason:       i.opts.replaceReason,
		RowsDeleted:  deleted,
		RowsInserted: inserted,
		BackupPath:   backupPath,
	}); err != nil {
		return inserted, err
	}
	return inserted, nil
}

func (i importer) fetchAndInsertTarget(ctx context.Context, target importTarget) (int64, error) {
	var insertedTotal int64
	windows := splitDateWindows(i.opts.fromDate, i.opts.toDate)
	for idx, window := range windows {
		from, to := sessionRange(window.FromDate, window.ToDate, i.opts.intradayLocation)
		fetchCtx, fetchCancel := context.WithTimeout(ctx, i.opts.timeout)
		result, err := i.client.FetchHistorical(fetchCtx, kite.HistoricalRequest{
			InstrumentToken: target.KiteInstrument.InstrumentToken,
			Interval:        kite.IntervalMinute,
			From:            from,
			To:              to,
		})
		fetchCancel()
		if err != nil {
			return insertedTotal, fmt.Errorf("fetch %s %s to %s: %w",
				target.RequestedSymbol, window.FromDate.Format(dateLayout), window.ToDate.Format(dateLayout), err)
		}
		if err := validateHistoricalResult(result); err != nil {
			return insertedTotal, fmt.Errorf("fetch %s %s to %s: %w",
				target.RequestedSymbol, window.FromDate.Format(dateLayout), window.ToDate.Format(dateLayout), err)
		}
		candles := kiteCandlesToModels(target.ModelInstrument.ID, result.Candles)
		insertCtx, insertCancel := context.WithTimeout(ctx, i.opts.timeout)
		inserted, err := i.candles.InsertBatchIgnoreDuplicates(insertCtx, candles)
		insertCancel()
		if err != nil {
			return insertedTotal, fmt.Errorf("insert candles for %s: %w", target.RequestedSymbol, err)
		}
		insertedTotal += inserted
		if idx < len(windows)-1 {
			if err := sleepContext(ctx, i.opts.delay); err != nil {
				return insertedTotal, err
			}
		}
	}
	return insertedTotal, nil
}

func (i importer) candleFilter(target importTarget) models.CandleFilter {
	from, to := sessionRange(i.opts.fromDate, i.opts.toDate, i.opts.intradayLocation)
	return models.CandleFilter{
		InstrumentID: target.ModelInstrument.ID,
		From:         from,
		To:           to,
		Interval:     models.CandleInterval1M,
	}
}

func validateHistoricalResult(result *kite.HistoricalResult) error {
	if result == nil {
		return fmt.Errorf("empty Kite historical result")
	}
	if result.HTTPStatus < http.StatusOK || result.HTTPStatus >= http.StatusMultipleChoices {
		return fmt.Errorf("Kite historical HTTP %d status=%s error_type=%s message=%s",
			result.HTTPStatus, result.APIStatus, result.ErrorType, result.Message)
	}
	if result.APIStatus != kite.APIStatusSuccess {
		return fmt.Errorf("Kite historical status=%s error_type=%s message=%s",
			result.APIStatus, result.ErrorType, result.Message)
	}
	return nil
}

func kiteCandlesToModels(instrumentID uuid.UUID, candles []kite.Candle) []models.Candle {
	result := make([]models.Candle, 0, len(candles))
	for _, candle := range candles {
		result = append(result, models.Candle{
			InstrumentID: instrumentID,
			Timestamp:    candle.Timestamp.UTC(),
			Interval:     models.CandleInterval1M,
			Open:         candle.Open,
			High:         candle.High,
			Low:          candle.Low,
			Close:        candle.Close,
			Volume:       candle.Volume,
			Provider:     providerZerodhaKite,
		})
	}
	return result
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
