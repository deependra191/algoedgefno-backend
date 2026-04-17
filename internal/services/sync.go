package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

type SyncService struct {
	syncRunStore *storage.SyncRunStore
	registry     *providers.Registry
}

func NewSyncService(
	syncRunStore *storage.SyncRunStore,
	registry *providers.Registry,
) *SyncService {
	return &SyncService{
		syncRunStore: syncRunStore,
		registry:     registry,
	}
}

func (s *SyncService) SyncProvider(ctx context.Context, providerName string) (*models.SyncRun, error) {
	p, ok := s.registry.Get(providerName)
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	runID := uuid.New()
	run := &models.SyncRun{
		ID:       runID,
		Provider: providerName,
		SyncType: models.SyncTypeFull,
		Status:   models.SyncRunRunning,
	}

	if err := s.syncRunStore.Create(ctx, run.ToEntity()); err != nil {
		return nil, errors.New("failed to create sync run")
	}

	instCount, instErr := p.SyncInstruments(ctx)
	if instErr != nil {
		return s.completeSyncRun(ctx, run, 0, instErr)
	}

	candleCount, candleErr := p.SyncCandles(ctx)
	if candleErr != nil {
		return s.completeSyncRun(ctx, run, instCount, candleErr)
	}

	return s.completeSyncRun(ctx, run, instCount+candleCount, nil)
}

func (s *SyncService) completeSyncRun(ctx context.Context, run *models.SyncRun, records int, syncErr error) (*models.SyncRun, error) {
	status := models.SyncRunCompleted
	var errMsg *string
	if syncErr != nil {
		status = models.SyncRunFailed
		msg := syncErr.Error()
		errMsg = &msg
	}

	if err := s.syncRunStore.Complete(ctx, run.ID, status, records, errMsg); err != nil {
		return nil, errors.New("failed to update sync run")
	}

	run.Status = status
	run.RecordsProcessed = records
	run.ErrorMessage = errMsg

	if syncErr != nil {
		return run, syncErr
	}
	return run, nil
}
