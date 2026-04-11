package models

import (
	"time"

	"github.com/google/uuid"
)

type SyncRun struct {
	ID               uuid.UUID  `json:"id"`
	Provider         string     `json:"provider"`
	SyncType         string     `json:"sync_type"`
	Status           string     `json:"status"`
	RecordsProcessed int        `json:"records_processed"`
	ErrorMessage     *string    `json:"error_message,omitempty"`
	StartedAt        time.Time  `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}
