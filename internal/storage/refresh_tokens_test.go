package storage

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

// TestToRefreshTokenModel_RoundTrip asserts that toRefreshTokenModel correctly
// maps all entities.RefreshToken fields to the domain model.
func TestToRefreshTokenModel_RoundTrip(t *testing.T) {
	now := time.Now().UTC()
	revokedAt := now.Add(-time.Hour)

	ent := &entities.RefreshToken{
		ID:        uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		UserID:    uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		TokenHash: "abc123hashvalue",
		ExpiresAt: now.Add(24 * time.Hour),
		RevokedAt: &revokedAt,
		CreatedAt: now,
	}

	m := toRefreshTokenModel(ent)

	if m.ID != ent.ID {
		t.Errorf("ID = %v, want %v", m.ID, ent.ID)
	}
	if m.UserID != ent.UserID {
		t.Errorf("UserID = %v, want %v", m.UserID, ent.UserID)
	}
	if m.TokenHash != ent.TokenHash {
		t.Errorf("TokenHash = %q, want %q", m.TokenHash, ent.TokenHash)
	}
	if !m.ExpiresAt.Equal(ent.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", m.ExpiresAt, ent.ExpiresAt)
	}
	if m.RevokedAt == nil || !m.RevokedAt.Equal(revokedAt) {
		t.Errorf("RevokedAt = %v, want %v", m.RevokedAt, revokedAt)
	}
	if !m.CreatedAt.Equal(ent.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", m.CreatedAt, ent.CreatedAt)
	}
}

// TestToRefreshTokenModel_NilRevokedAt asserts that nil RevokedAt maps to nil
// on the domain model (token is still active).
func TestToRefreshTokenModel_NilRevokedAt(t *testing.T) {
	ent := &entities.RefreshToken{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: "hash",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		RevokedAt: nil,
		CreatedAt: time.Now().UTC(),
	}
	m := toRefreshTokenModel(ent)
	if m.RevokedAt != nil {
		t.Errorf("RevokedAt = %v, want nil for active token", m.RevokedAt)
	}
}
