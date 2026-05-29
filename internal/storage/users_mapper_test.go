package storage

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

// TestToUserModel_RoundTrip asserts that toUserModel correctly maps all
// entities.User fields to the domain model. In particular, FirebaseUID must
// always be non-empty (post-017 invariant) and nullable string fields map to
// empty strings on the domain object when nil.
func TestToUserModel_RoundTrip(t *testing.T) {
	displayName := "Test User"
	photoURL := "https://example.com/photo.jpg"
	now := time.Now().UTC()

	ent := &entities.User{
		ID:          uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email:       "user@example.com",
		FirebaseUID: "firebase-uid-abc",
		DisplayName: &displayName,
		PhotoURL:    &photoURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m := toUserModel(ent)

	if m.ID != ent.ID {
		t.Errorf("ID = %v, want %v", m.ID, ent.ID)
	}
	if m.FirebaseUID != ent.FirebaseUID {
		t.Errorf("FirebaseUID = %q, want %q", m.FirebaseUID, ent.FirebaseUID)
	}
	if m.Email != ent.Email {
		t.Errorf("Email = %q, want %q", m.Email, ent.Email)
	}
	if m.DisplayName != displayName {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, displayName)
	}
	if m.PhotoURL != photoURL {
		t.Errorf("PhotoURL = %q, want %q", m.PhotoURL, photoURL)
	}
}

// TestToUserModel_NilOptionals asserts that nil DisplayName and PhotoURL map
// to empty strings on the domain model (not panics or zero pointers).
func TestToUserModel_NilOptionals(t *testing.T) {
	ent := &entities.User{
		ID:          uuid.New(),
		Email:       "user@example.com",
		FirebaseUID: "uid-xyz",
		DisplayName: nil,
		PhotoURL:    nil,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	m := toUserModel(ent)
	if m.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty string for nil entity field", m.DisplayName)
	}
	if m.PhotoURL != "" {
		t.Errorf("PhotoURL = %q, want empty string for nil entity field", m.PhotoURL)
	}
	if m.FirebaseUID == "" {
		t.Error("FirebaseUID must not be empty (post-017 NOT NULL invariant)")
	}
}
