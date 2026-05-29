package entities

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken is the DB row for the refresh_tokens table (migration 018).
// RevokedAt is nullable — NULL means the token is still active.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}
