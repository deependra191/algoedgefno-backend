package entities

import (
	"time"

	"github.com/google/uuid"
)

// SyncRun is the DB row for the sync_runs table.
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
