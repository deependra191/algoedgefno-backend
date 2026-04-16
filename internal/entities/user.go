// Package entities contains DB-facing structs used as pgx scan targets.
// Fields mirror the DB schema exactly. Entities must not carry `json:` tags
// and must not be serialized to HTTP responses — that is the job of
// handler-local response DTOs. Services convert entities to domain models
// via the mappers in internal/models.
package entities

import (
	"time"

	"github.com/google/uuid"
)

// User is the DB row for the users table.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
