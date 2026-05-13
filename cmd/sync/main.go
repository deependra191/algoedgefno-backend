package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/buildinfo"
	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
	"github.com/deependra191/algoedgefno-backend/internal/logging"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/providers/nse"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

const (
	dateLayout   = "2006-01-02"
	syncTimeout  = 5 * time.Minute
	queryTimeout = 30 * time.Second
)

func main() {
	dateStr := flag.String("date", "", "target date in YYYY-MM-DD format (default: latest trading day)")
	fromStr := flag.String("from", "", "backfill start date in YYYY-MM-DD format")
	toStr := flag.String("to", "", "backfill end date in YYYY-MM-DD format (default: today)")
	delaySec := flag.Int("delay", 2, "seconds to sleep between backfill requests")
	flag.Parse()

	cfg := config.Load()

	slog.SetDefault(slog.New(logging.NewHandler(cfg.Env, os.Stderr)).With(
		"env", string(cfg.Env),
		"version", buildinfo.AppVersion,
		"commit", buildinfo.CommitSHA,
	))

	if err := cfg.ValidateStartupIdentity(); err != nil {
		slog.Error("startup validation failed", "error", err)
		os.Exit(1)
	}

	if !cfg.SyncEnabled {
		slog.Info("sync disabled via SYNC_ENABLED=false, exiting")
		return
	}

	pool := database.Connect(cfg)
	defer pool.Close()

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)
	syncRunStore := storage.NewSyncRunStore(pool)

	if *fromStr != "" {
		from, err := time.Parse(dateLayout, *fromStr)
		if err != nil {
			slog.Error("invalid -from date", "value", *fromStr, "error", err)
			os.Exit(1)
		}
		to := time.Now().UTC().Truncate(24 * time.Hour)
		if *toStr != "" {
			to, err = time.Parse(dateLayout, *toStr)
			if err != nil {
				slog.Error("invalid -to date", "value", *toStr, "error", err)
				os.Exit(1)
			}
		}

		slog.Info("backfilling NSE EOD data", "from", from.Format(dateLayout), "to", to.Format(dateLayout))
		var totalRecords, succeeded, failed int
		var failedDays []string
		for day := from; !day.After(to); day = nextTradingDay(day) {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			run, err := syncForDate(ctx, instrumentStore, candleStore, syncRunStore, cfg, day)
			cancel()
			if err != nil {
				failed++
				failedDays = append(failedDays, day.Format(dateLayout))
				slog.Warn("sync failed for date, skipping", "date", day.Format(dateLayout), "error", err)
			} else {
				succeeded++
				totalRecords += run.RecordsProcessed
				slog.Info("synced date", "date", day.Format(dateLayout), "records", run.RecordsProcessed, "total_records", totalRecords)
			}
			time.Sleep(time.Duration(*delaySec) * time.Second)
		}
		slog.Info("backfill complete", "days_synced", succeeded, "days_failed", failed, "total_records", totalRecords)
		if len(failedDays) > 0 {
			slog.Info("failed days (likely holidays)", "days", failedDays)
		}
		return
	}

	if *dateStr != "" {
		// Manual single-day override.
		t, err := time.Parse(dateLayout, *dateStr)
		if err != nil {
			slog.Error("invalid -date value", "value", *dateStr, "error", err)
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		slog.Info("syncing NSE EOD data", "date", *dateStr)
		run, err := syncForDate(ctx, instrumentStore, candleStore, syncRunStore, cfg, t)
		if err != nil {
			slog.Error("sync failed", "error", err)
			os.Exit(1)
		}
		slog.Info("sync complete", "records_processed", run.RecordsProcessed)
		return
	}

	// Daily catchup mode: sync every calendar day from the last synced date to today.
	// Running on weekdays only (via cron); Monday naturally catches Saturday and Sunday.
	// Weekends and holidays 404 and are skipped — no silent walk-back.
	queryCtx, queryCancel := context.WithTimeout(context.Background(), queryTimeout)
	lastSynced, err := candleStore.LastSyncedDate(queryCtx, nse.ProviderName)
	queryCancel()
	if err != nil {
		slog.Error("failed to get last synced date", "error", err)
		os.Exit(1)
	}
	if lastSynced.IsZero() {
		slog.Error("no data found", "hint", "run a backfill first: go run ./cmd/sync -from YYYY-MM-DD")
		os.Exit(1)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	if !lastSynced.Before(today) {
		slog.Info("already up to date, nothing to sync")
		return
	}

	slog.Info("daily catchup starting", "last_synced", lastSynced.Format(dateLayout), "syncing_to", today.Format(dateLayout))
	var totalRecords, succeeded, failed int
	var failedDays []string
	for day := lastSynced.AddDate(0, 0, 1); !day.After(today); day = day.AddDate(0, 0, 1) {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		run, err := syncForDate(ctx, instrumentStore, candleStore, syncRunStore, cfg, day)
		cancel()
		if err != nil {
			failed++
			failedDays = append(failedDays, day.Format(dateLayout))
			slog.Warn("sync failed for date, skipping", "date", day.Format(dateLayout), "error", err)
		} else {
			succeeded++
			totalRecords += run.RecordsProcessed
			slog.Info("synced date", "date", day.Format(dateLayout), "records", run.RecordsProcessed)
		}
		time.Sleep(time.Duration(*delaySec) * time.Second)
	}
	slog.Info("daily catchup complete", "days_synced", succeeded, "days_failed", failed, "total_records", totalRecords)
	if len(failedDays) > 0 {
		slog.Info("failed days (holidays/weekends)", "days", failedDays)
	}
}

// syncForDate runs one sync cycle for the given date (zero value = latest trading day).
func syncForDate(
	ctx context.Context,
	instrumentStore *storage.InstrumentStore,
	candleStore *storage.CandleStore,
	syncRunStore *storage.SyncRunStore,
	cfg *config.Config,
	date time.Time,
	extraOpts ...nse.EODOption,
) (*models.SyncRun, error) {
	opts := []nse.EODOption{
		nse.WithUserAgent(cfg.NSEUserAgent),
		nse.WithAcceptHTML(cfg.NSEAcceptHTML),
	}
	if !date.IsZero() {
		opts = append(opts, nse.WithTargetDate(date))
	}
	opts = append(opts, extraOpts...)
	registry := providers.NewRegistry()
	registry.Register(nse.NewEODProvider(instrumentStore, candleStore, opts...))
	svc := services.NewSyncService(syncRunStore, registry)
	return svc.SyncProvider(ctx, nse.ProviderName)
}

// nextTradingDay advances to the next weekday (Mon–Fri), skipping weekends.
// NSE holidays result in a 404 from the archive server; the provider logs and
// the caller skips to the next iteration.
func nextTradingDay(t time.Time) time.Time {
	t = t.AddDate(0, 0, 1)
	switch t.Weekday() {
	case time.Saturday:
		t = t.AddDate(0, 0, 2)
	case time.Sunday:
		t = t.AddDate(0, 0, 1)
	}
	return t
}
