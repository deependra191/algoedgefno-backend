package models

import (
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

const (
	SyncRunPending   = "PENDING"
	SyncRunRunning   = "RUNNING"
	SyncRunCompleted = "COMPLETED"
	SyncRunFailed    = "FAILED"
)

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
