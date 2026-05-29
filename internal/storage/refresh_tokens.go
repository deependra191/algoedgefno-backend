package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

var _ models.RefreshTokenRepository = (*RefreshTokenStore)(nil)

// RefreshTokenStore implements models.RefreshTokenRepository using a
// pgxpool.Pool.
type RefreshTokenStore struct {
	pool *pgxpool.Pool
}

// NewRefreshTokenStore constructs a RefreshTokenStore backed by the given
// connection pool.
func NewRefreshTokenStore(pool *pgxpool.Pool) *RefreshTokenStore {
	return &RefreshTokenStore{pool: pool}
}

// Create inserts a new refresh-token row. The token_hash must already be
// computed by the caller (sha256HexOf the raw base64url token).
func (s *RefreshTokenStore) Create(ctx context.Context, t *models.RefreshToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

// LookupActiveUserID returns the user_id for the given token hash if the
// token exists, is not revoked, and has not expired. Returns models.ErrNotFound
// when no active row matches.
func (s *RefreshTokenStore) LookupActiveUserID(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var uid uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM refresh_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()`,
		tokenHash,
	).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, models.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup active user id: %w", err)
	}
	return uid, nil
}

// RotateRefreshToken atomically revokes the old token (identified by oldHash)
// and inserts the new token in a single transaction. Returns models.ErrNotFound
// when 0 rows match oldHash (concurrent rotation already consumed it).
func (s *RefreshTokenStore) RotateRefreshToken(ctx context.Context, oldHash string, newToken *models.RefreshToken) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("rotate refresh token begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()
		RETURNING user_id`,
		oldHash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, models.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("rotate refresh token revoke old: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		newToken.ID, newToken.UserID, newToken.TokenHash, newToken.ExpiresAt,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("rotate refresh token insert new: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("rotate refresh token commit: %w", err)
	}
	return userID, nil
}

// RevokeByHash marks the token row matching tokenHash as revoked by setting
// revoked_at = NOW(). Idempotent — an absent or already-revoked token returns
// nil (0 rows affected is not an error per the SQL contract).
func (s *RefreshTokenStore) RevokeByHash(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf("revoke by hash: %w", err)
	}
	return nil
}

// DeleteExpired removes all rows whose expires_at is in the past and returns
// the count of deleted rows. Used by the nightly cleanup cron.
func (s *RefreshTokenStore) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

// toRefreshTokenModel converts an entities.RefreshToken scan target to the
// domain model. RevokedAt is nil when the token is still active.
func toRefreshTokenModel(e *entities.RefreshToken) *models.RefreshToken {
	return &models.RefreshToken{
		ID:        e.ID,
		UserID:    e.UserID,
		TokenHash: e.TokenHash,
		ExpiresAt: e.ExpiresAt,
		RevokedAt: e.RevokedAt,
		CreatedAt: e.CreatedAt,
	}
}
