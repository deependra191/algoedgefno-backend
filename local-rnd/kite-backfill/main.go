package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
	"github.com/deependra191/algoedgefno-backend/local-rnd/kite"
)

func main() {
	os.Exit(run())
}

func run() int {
	now := time.Now()
	opts, err := parseOptions(os.Args[1:], now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse options: %v\n", err)
		return exitError
	}

	cfg := config.Load()
	cfg.AutoMigrate = false
	if err := validateLocalConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "local guard: %v\n", err)
		return exitError
	}

	pool := database.Connect(cfg)
	defer pool.Close()

	guardCtx, guardCancel := context.WithTimeout(context.Background(), opts.timeout)
	if err := ensureLocalDatabaseIdentity(guardCtx, pool); err != nil {
		guardCancel()
		fmt.Fprintf(os.Stderr, "local database guard: %v\n", err)
		return exitError
	}
	guardCancel()

	creds, err := kite.LoadReadOnlyCredentials()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load Kite credentials: %v\n", err)
		return exitError
	}
	client := kite.NewClient(creds)

	instrumentsCtx, instrumentsCancel := context.WithTimeout(context.Background(), opts.timeout)
	kiteInstruments, rawCSV, err := client.InstrumentDump(instrumentsCtx)
	instrumentsCancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch Kite instruments: %v\n", err)
		return exitError
	}
	snapshotPath, err := writeInstrumentSnapshot(rawCSV, opts.snapshotDir, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write Kite instruments snapshot: %v\n", err)
		return exitError
	}

	resolver := newInstrumentResolver(kiteInstruments, now.In(opts.intradayLocation))
	targets, err := resolveTargets(resolver, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve targets: %v\n", err)
		return exitError
	}

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)
	syncRunStore := storage.NewSyncRunStore(pool)

	runID := uuid.New()
	syncType := models.SyncTypeFull
	if opts.replace {
		syncType = models.SyncTypeReplace
	}
	syncRun := &models.SyncRun{
		ID:       runID,
		Provider: providerZerodhaKite,
		SyncType: syncType,
		Status:   models.SyncRunRunning,
	}
	createCtx, createCancel := context.WithTimeout(context.Background(), opts.timeout)
	if err := syncRunStore.Create(createCtx, syncRun); err != nil {
		createCancel()
		fmt.Fprintf(os.Stderr, "create sync run: %v\n", err)
		return exitError
	}
	createCancel()

	targets, err = upsertAndAttachTargets(context.Background(), opts.timeout, instrumentStore, targets)
	if err != nil {
		completeCtx, completeCancel := context.WithTimeout(context.Background(), opts.timeout)
		completeErr := completeSyncRun(completeCtx, syncRunStore, runID, 0, err)
		completeCancel()
		if completeErr != nil {
			fmt.Fprintf(os.Stderr, "complete failed sync run: %v\n", completeErr)
		}
		fmt.Fprintf(os.Stderr, "upsert instruments: %v\n", err)
		return exitError
	}

	runner := importer{
		client:  client,
		candles: candleStore,
		opts:    opts,
		runID:   runID,
	}
	inserted, importErr := runner.importTargets(context.Background(), targets)

	completeCtx, completeCancel := context.WithTimeout(context.Background(), opts.timeout)
	completeErr := completeSyncRun(completeCtx, syncRunStore, runID, inserted, importErr)
	completeCancel()
	if completeErr != nil {
		fmt.Fprintf(os.Stderr, "complete sync run: %v\n", completeErr)
		return exitError
	}
	if importErr != nil {
		fmt.Fprintf(os.Stderr, "kite backfill failed: run_id=%s rows_inserted=%d error=%v\n", runID, inserted, importErr)
		return exitError
	}

	fmt.Printf("kite backfill complete: run_id=%s targets=%d rows_inserted=%d instruments_snapshot=%s\n",
		runID, len(targets), inserted, snapshotPath)
	return exitOK
}

func upsertAndAttachTargets(
	parent context.Context,
	timeout time.Duration,
	instrumentStore *storage.InstrumentStore,
	targets []importTarget,
) ([]importTarget, error) {
	modelInstruments := make([]models.Instrument, 0, len(targets))
	for _, target := range targets {
		modelInstruments = append(modelInstruments, target.ModelInstrument)
	}

	upsertCtx, upsertCancel := context.WithTimeout(parent, timeout)
	if err := instrumentStore.UpsertBatch(upsertCtx, modelInstruments); err != nil {
		upsertCancel()
		return nil, err
	}
	upsertCancel()

	listCtx, listCancel := context.WithTimeout(parent, timeout)
	persisted, err := instrumentStore.List(listCtx, models.InstrumentFilter{})
	listCancel()
	if err != nil {
		return nil, err
	}
	return attachPersistedInstruments(targets, persisted)
}

func completeSyncRun(
	ctx context.Context,
	syncRunStore *storage.SyncRunStore,
	runID uuid.UUID,
	inserted int64,
	importErr error,
) error {
	status := models.SyncRunCompleted
	var errMsg *string
	if importErr != nil {
		status = models.SyncRunFailed
		msg := importErr.Error()
		errMsg = &msg
	}
	return syncRunStore.Complete(ctx, runID, status, int(inserted), errMsg)
}
