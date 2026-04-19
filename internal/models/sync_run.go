package models

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

// SyncRun is the domain representation of a provider data sync attempt.
type SyncRun struct {
	ID               uuid.UUID
	Provider         string
	SyncType         string
	Status           string
	RecordsProcessed int
	ErrorMessage     *string
	StartedAt        time.Time
	CompletedAt      *time.Time
}

// SyncRun status values represent the lifecycle of a sync attempt.
const (
	SyncRunPending   = "PENDING"
	SyncRunRunning   = "RUNNING"
	SyncRunCompleted = "COMPLETED"
	SyncRunFailed    = "FAILED"
)

// SyncRunRepository is the storage contract for persisting sync run records.
type SyncRunRepository interface {
	// Create inserts a new sync run record with RUNNING status.
	Create(ctx context.Context, run *SyncRun) error
	// Complete updates the sync run to its final status (COMPLETED or FAILED) with record count and optional error.
	Complete(ctx context.Context, id uuid.UUID, status string, records int, errMsg *string) error
}

// SyncTypeFull is the only sync type currently implemented — a complete bhavcopy
// download covering all instruments and candles for a given day.
// An incremental type may be added if NSE exposes delta feeds in future.
const SyncTypeFull = "full"

func FromSyncRunEntity(e *entities.SyncRun) *SyncRun {
	if e == nil {
		return nil
	}
	return &SyncRun{
		ID:               e.ID,
		Provider:         e.Provider,
		SyncType:         e.SyncType,
		Status:           e.Status,
		RecordsProcessed: e.RecordsProcessed,
		ErrorMessage:     e.ErrorMessage,
		StartedAt:        e.StartedAt,
		CompletedAt:      e.CompletedAt,
	}
}

func (r *SyncRun) ToEntity() *entities.SyncRun {
	if r == nil {
		return nil
	}
	return &entities.SyncRun{
		ID:               r.ID,
		Provider:         r.Provider,
		SyncType:         r.SyncType,
		Status:           r.Status,
		RecordsProcessed: r.RecordsProcessed,
		ErrorMessage:     r.ErrorMessage,
		StartedAt:        r.StartedAt,
		CompletedAt:      r.CompletedAt,
	}
}
