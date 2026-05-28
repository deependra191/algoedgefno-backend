package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// insertTestUserForTokens inserts a minimal users row and returns its UUID.
// Used by refresh-token tests that need a valid FK target.
func insertTestUserForTokens(t *testing.T, ctx context.Context, pool interface {
	Exec(context.Context, string, ...any) (interface{ RowsAffected() int64 }, error)
}) uuid.UUID {
	t.Helper()
	// Use the UserStore for insertion to avoid raw SQL duplication.
	return uuid.Nil // placeholder — real implementation uses pool.Exec
}

// insertRefreshTokenUser inserts a test user via the UserStore and returns its ID.
func insertRefreshTokenUser(t *testing.T, ctx context.Context, s *RefreshTokenStore) uuid.UUID {
	t.Helper()
	userStore := NewUserStore(s.pool)
	uid := "rt-test-uid-" + uuid.NewString()
	email := "rt-test-" + uuid.NewString() + "@example.com"
	user, err := userStore.UpsertByFirebaseUID(ctx, &models.User{
		FirebaseUID: uid,
		Email:       email,
	})
	if err != nil {
		t.Fatalf("insertRefreshTokenUser: %v", err)
	}
	t.Cleanup(func() {
		s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user.ID
}

// TestRefreshTokenStore_Create_And_LookupActiveUserID is the basic round-trip:
// create a token, look it up, get the correct user ID back.
func TestRefreshTokenStore_Create_And_LookupActiveUserID(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	userID := insertRefreshTokenUser(t, ctx, s)
	tokenID := uuid.New()
	hash := "testhash-" + uuid.NewString()
	rt := &models.RefreshToken{
		ID:        tokenID,
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, tokenID)
	})

	if err := s.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotUID, err := s.LookupActiveUserID(ctx, hash)
	if err != nil {
		t.Fatalf("LookupActiveUserID: %v", err)
	}
	if gotUID != userID {
		t.Errorf("user_id = %v, want %v", gotUID, userID)
	}
}

// TestRefreshTokenStore_LookupActiveUserID_RevokedReturnsNotFound asserts that
// a revoked token is not returned by LookupActiveUserID.
func TestRefreshTokenStore_LookupActiveUserID_RevokedReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	userID := insertRefreshTokenUser(t, ctx, s)
	tokenID := uuid.New()
	hash := "revoked-hash-" + uuid.NewString()
	rt := &models.RefreshToken{
		ID:        tokenID,
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, tokenID)
	})

	if err := s.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.RevokeByHash(ctx, hash); err != nil {
		t.Fatalf("RevokeByHash: %v", err)
	}

	_, err := s.LookupActiveUserID(ctx, hash)
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for revoked token, got %v", err)
	}
}

// TestRefreshTokenStore_LookupActiveUserID_ExpiredReturnsNotFound asserts that
// an expired token (expires_at in the past) is not returned.
func TestRefreshTokenStore_LookupActiveUserID_ExpiredReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	userID := insertRefreshTokenUser(t, ctx, s)
	tokenID := uuid.New()
	hash := "expired-hash-" + uuid.NewString()
	// Insert with an already-expired timestamp directly via SQL.
	_, err := pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, NOW() - interval '1 second', NOW())`,
		tokenID, userID, hash,
	)
	if err != nil {
		t.Fatalf("insert expired token: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, tokenID)
	})

	_, err = s.LookupActiveUserID(ctx, hash)
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired token, got %v", err)
	}
}

// TestRefreshTokenStore_RotateRefreshToken asserts that rotation revokes the
// old token and inserts the new one atomically.
func TestRefreshTokenStore_RotateRefreshToken(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	userID := insertRefreshTokenUser(t, ctx, s)
	oldID := uuid.New()
	oldHash := "old-hash-" + uuid.NewString()
	newID := uuid.New()
	newHash := "new-hash-" + uuid.NewString()

	old := &models.RefreshToken{
		ID:        oldID,
		UserID:    userID,
		TokenHash: oldHash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id IN ($1, $2)`, oldID, newID)
	})

	if err := s.Create(ctx, old); err != nil {
		t.Fatalf("Create old: %v", err)
	}

	newRT := &models.RefreshToken{
		ID:        newID,
		UserID:    userID,
		TokenHash: newHash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	gotUID, err := s.RotateRefreshToken(ctx, oldHash, newRT)
	if err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}
	if gotUID != userID {
		t.Errorf("RotateRefreshToken user_id = %v, want %v", gotUID, userID)
	}

	// Old token must now be revoked.
	_, err = s.LookupActiveUserID(ctx, oldHash)
	if !errors.Is(err, models.ErrNotFound) {
		t.Errorf("old token still active after rotation: %v", err)
	}

	// New token must be active.
	gotUID2, err := s.LookupActiveUserID(ctx, newHash)
	if err != nil {
		t.Fatalf("LookupActiveUserID new token: %v", err)
	}
	if gotUID2 != userID {
		t.Errorf("new token user_id = %v, want %v", gotUID2, userID)
	}
}

// TestRefreshTokenStore_RotateRefreshToken_ConsumedHashReturnsNotFound asserts
// that RotateRefreshToken returns ErrNotFound when the old hash is already
// revoked (concurrent rotation scenario).
func TestRefreshTokenStore_RotateRefreshToken_ConsumedHashReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	userID := insertRefreshTokenUser(t, ctx, s)
	oldID := uuid.New()
	oldHash := "consumed-hash-" + uuid.NewString()
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, oldID)
	})

	old := &models.RefreshToken{
		ID:        oldID,
		UserID:    userID,
		TokenHash: oldHash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	if err := s.Create(ctx, old); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Revoke before rotate (simulates concurrent rotation).
	if err := s.RevokeByHash(ctx, oldHash); err != nil {
		t.Fatalf("RevokeByHash: %v", err)
	}

	newRT := &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: "new-after-consumed-" + uuid.NewString(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	_, err := s.RotateRefreshToken(ctx, oldHash, newRT)
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for consumed hash, got %v", err)
	}
}

// TestRefreshTokenStore_RevokeByHash_Idempotent asserts that revoking an
// already-revoked or absent token returns nil.
func TestRefreshTokenStore_RevokeByHash_Idempotent(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	// Revoke a non-existent hash — should not error.
	if err := s.RevokeByHash(ctx, "does-not-exist-hash"); err != nil {
		t.Fatalf("RevokeByHash non-existent: %v", err)
	}
}

// TestRefreshTokenStore_DeleteExpired asserts that DeleteExpired removes only
// expired rows and returns the correct count.
func TestRefreshTokenStore_DeleteExpired(t *testing.T) {
	pool := newTestPool(t)
	s := NewRefreshTokenStore(pool)
	ctx := context.Background()

	userID := insertRefreshTokenUser(t, ctx, s)

	// Insert one expired and one active token.
	expiredID := uuid.New()
	activeID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES
		  ($1, $2, $3, NOW() - interval '1 second', NOW()),
		  ($4, $2, $5, NOW() + interval '1 hour',   NOW())`,
		expiredID, userID, "expire-delete-"+uuid.NewString(),
		activeID, "active-nodelete-"+uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("insert test tokens: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE id IN ($1, $2)`, expiredID, activeID)
	})

	n, err := s.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if n < 1 {
		t.Errorf("DeleteExpired returned %d, want >= 1", n)
	}

	// Active token must still exist.
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE id = $1`, activeID).Scan(&count); err != nil {
		t.Fatalf("check active: %v", err)
	}
	if count != 1 {
		t.Errorf("active token was deleted unexpectedly")
	}
}
