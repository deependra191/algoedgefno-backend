package models

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RefreshToken is the domain representation of an issued refresh-token row.
// TokenHash stores the SHA-256 hex digest of the raw base64url token; the raw
// value is returned to Android at issuance and never persisted.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// RefreshTokenRepository is the storage contract for refresh-token persistence.
type RefreshTokenRepository interface {
	// Create inserts a new refresh-token row.
	Create(ctx context.Context, t *RefreshToken) error

	// LookupActiveUserID returns the user_id for the given token hash if the
	// token exists, is not revoked, and has not expired. Returns ErrNotFound
	// when no active row matches.
	LookupActiveUserID(ctx context.Context, tokenHash string) (uuid.UUID, error)

	// RotateRefreshToken atomically marks the old token (identified by
	// oldHash) as revoked and inserts the newToken row in a single
	// transaction. Returns the user_id of the consumed token. Returns
	// ErrNotFound when 0 rows match the oldHash (already consumed by a
	// concurrent rotation or never existed).
	RotateRefreshToken(ctx context.Context, oldHash string, newToken *RefreshToken) (uuid.UUID, error)

	// RevokeByHash marks the token row matching tokenHash as revoked by
	// setting revoked_at = NOW(). Idempotent — an absent or already-revoked
	// token returns nil (0 rows affected is not an error).
	RevokeByHash(ctx context.Context, tokenHash string) error

	// DeleteExpired removes all rows whose expires_at is in the past and
	// returns the number of rows deleted. Used by the nightly cleanup cron.
	DeleteExpired(ctx context.Context) (int64, error)
}
