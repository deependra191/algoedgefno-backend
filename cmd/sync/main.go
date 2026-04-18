package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/providers/nse"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

func main() {
	dateStr := flag.String("date", "", "target date in YYYY-MM-DD format (default: latest trading day)")
	flag.Parse()

	cfg := config.Load()

	pool := database.Connect(cfg)
	defer pool.Close()

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)
	syncRunStore := storage.NewSyncRunStore(pool)

	var opts []nse.EODOption
	if *dateStr != "" {
		t, err := time.Parse("2006-01-02", *dateStr)
		if err != nil {
			log.Fatalf("invalid date %q: %v", *dateStr, err)
		}
		opts = append(opts, nse.WithTargetDate(t))
	}

	registry := providers.NewRegistry()
	registry.Register(nse.NewEODProvider(instrumentStore, candleStore, opts...))

	syncService := services.NewSyncService(syncRunStore, registry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Println("syncing NSE EOD data...")
	run, err := syncService.SyncProvider(ctx, nse.ProviderName)
	if err != nil {
		log.Fatalf("sync failed: %v", err)
	}

	log.Printf("sync completed: %d records processed", run.RecordsProcessed)
}
