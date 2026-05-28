// Package entities contains DB-facing structs used as pgx scan targets.
// Fields mirror the DB schema exactly. Entities must not carry `json:` tags
// and must not be serialized to HTTP responses — that is the job of
// handler-local response DTOs. Storage converts entities to domain models
// via private mappers inside the relevant storage file; entities never leave
// the storage package.
package entities

import (
	"time"

	"github.com/google/uuid"
)

// User is the DB row for the users table (post-migration-017).
// FirebaseUID is NOT NULL after migration 017; Name and PasswordHash are
// nullable. LastLoginAt is nullable (updated on each Firebase-verified login).
// UpdatedAt is non-nullable and maintained by every write path.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         *string
	PasswordHash *string
	FirebaseUID  string
	DisplayName  *string
	PhotoURL     *string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
