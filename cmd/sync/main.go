package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
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

	pool := database.Connect(cfg)
	defer pool.Close()

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)
	syncRunStore := storage.NewSyncRunStore(pool)

	if *fromStr != "" {
		from, err := time.Parse(dateLayout, *fromStr)
		if err != nil {
			log.Fatalf("invalid -from date %q: %v", *fromStr, err)
		}
		to := time.Now().UTC().Truncate(24 * time.Hour)
		if *toStr != "" {
			to, err = time.Parse(dateLayout, *toStr)
			if err != nil {
				log.Fatalf("invalid -to date %q: %v", *toStr, err)
			}
		}

		log.Printf("backfilling NSE EOD data from %s to %s...", from.Format(dateLayout), to.Format(dateLayout))
		var totalRecords, succeeded, failed int
		var failedDays []string
		for day := from; !day.After(to); day = nextTradingDay(day) {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			run, err := syncForDate(ctx, instrumentStore, candleStore, syncRunStore, day)
			cancel()
			if err != nil {
				failed++
				failedDays = append(failedDays, day.Format(dateLayout))
				log.Printf("[%s] failed: %v — skipping", day.Format(dateLayout), err)
			} else {
				succeeded++
				totalRecords += run.RecordsProcessed
				log.Printf("[%s] synced %d records (running total: %d)", day.Format(dateLayout), run.RecordsProcessed, totalRecords)
			}
			time.Sleep(time.Duration(*delaySec) * time.Second)
		}
		log.Printf("backfill complete: %d days synced, %d days failed (likely holidays), %d total records", succeeded, failed, totalRecords)
		if len(failedDays) > 0 {
			log.Printf("failed days: %v", failedDays)
		}
		return
	}

	if *dateStr != "" {
		// Manual single-day override.
		t, err := time.Parse(dateLayout, *dateStr)
		if err != nil {
			log.Fatalf("invalid date %q: %v", *dateStr, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		log.Printf("syncing NSE EOD data for %s...", *dateStr)
		run, err := syncForDate(ctx, instrumentStore, candleStore, syncRunStore, t)
		if err != nil {
			log.Fatalf("sync failed: %v", err)
		}
		log.Printf("sync completed: %d records processed", run.RecordsProcessed)
		return
	}

	// Daily catchup mode: sync every calendar day from the last synced date to today.
	// Running on weekdays only (via cron); Monday naturally catches Saturday and Sunday.
	// Weekends and holidays 404 and are skipped — no silent walk-back.
	queryCtx, queryCancel := context.WithTimeout(context.Background(), queryTimeout)
	lastSynced, err := candleStore.LastSyncedDate(queryCtx, nse.ProviderName)
	queryCancel()
	if err != nil {
		log.Fatalf("failed to get last synced date: %v", err)
	}
	if lastSynced.IsZero() {
		log.Fatal("no data found — run a backfill first: go run ./cmd/sync -from YYYY-MM-DD")
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	if !lastSynced.Before(today) {
		log.Println("already up to date, nothing to sync")
		return
	}

	log.Printf("daily catchup: last synced %s, syncing to %s...", lastSynced.Format(dateLayout), today.Format(dateLayout))
	var totalRecords, succeeded, failed int
	var failedDays []string
	for day := lastSynced.AddDate(0, 0, 1); !day.After(today); day = day.AddDate(0, 0, 1) {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		run, err := syncForDate(ctx, instrumentStore, candleStore, syncRunStore, day)
		cancel()
		if err != nil {
			failed++
			failedDays = append(failedDays, day.Format(dateLayout))
			log.Printf("[%s] failed: %v — skipping", day.Format(dateLayout), err)
		} else {
			succeeded++
			totalRecords += run.RecordsProcessed
			log.Printf("[%s] synced %d records", day.Format(dateLayout), run.RecordsProcessed)
		}
		time.Sleep(time.Duration(*delaySec) * time.Second)
	}
	log.Printf("daily catchup complete: %d days synced, %d days failed (holidays/weekends), %d total records", succeeded, failed, totalRecords)
	if len(failedDays) > 0 {
		log.Printf("failed days: %v", failedDays)
	}
}

// syncForDate runs one sync cycle for the given date (zero value = latest trading day).
func syncForDate(
	ctx context.Context,
	instrumentStore *storage.InstrumentStore,
	candleStore *storage.CandleStore,
	syncRunStore *storage.SyncRunStore,
	date time.Time,
	extraOpts ...nse.EODOption,
) (*models.SyncRun, error) {
	opts := extraOpts
	if !date.IsZero() {
		opts = append([]nse.EODOption{nse.WithTargetDate(date)}, extraOpts...)
	}
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
