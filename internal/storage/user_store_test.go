package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// TestUserStore_UpsertByFirebaseUID_Insert asserts that a brand-new
// (firebase_uid, email) pair results in a new users row.
func TestUserStore_UpsertByFirebaseUID_Insert(t *testing.T) {
	pool := newTestPool(t)
	store := NewUserStore(pool)
	ctx := context.Background()

	in := &models.User{
		FirebaseUID: "uid-upsert-insert-" + uuid.NewString(),
		Email:       "insert-" + uuid.NewString() + "@example.com",
		DisplayName: "Insert Test",
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE firebase_uid = $1`, in.FirebaseUID)

	out, err := store.UpsertByFirebaseUID(ctx, in)
	if err != nil {
		t.Fatalf("UpsertByFirebaseUID (insert): %v", err)
	}
	if out.ID == uuid.Nil {
		t.Error("returned user has nil ID")
	}
	if out.FirebaseUID != in.FirebaseUID {
		t.Errorf("FirebaseUID = %q, want %q", out.FirebaseUID, in.FirebaseUID)
	}
	if out.Email != canonicalizeEmail(in.Email) {
		t.Errorf("Email = %q, want %q", out.Email, canonicalizeEmail(in.Email))
	}
}

// TestUserStore_UpsertByFirebaseUID_Update asserts that an existing
// firebase_uid row is updated on subsequent upserts.
func TestUserStore_UpsertByFirebaseUID_Update(t *testing.T) {
	pool := newTestPool(t)
	store := NewUserStore(pool)
	ctx := context.Background()

	uid := "uid-upsert-update-" + uuid.NewString()
	defer pool.Exec(ctx, `DELETE FROM users WHERE firebase_uid = $1`, uid)

	in := &models.User{FirebaseUID: uid, Email: "orig@example.com", DisplayName: "Orig"}
	first, err := store.UpsertByFirebaseUID(ctx, in)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	in2 := &models.User{FirebaseUID: uid, Email: "orig@example.com", DisplayName: "Updated"}
	second, err := store.UpsertByFirebaseUID(ctx, in2)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("ID changed on update: got %v, want %v", second.ID, first.ID)
	}
	if second.DisplayName != "Updated" {
		t.Errorf("DisplayName = %q, want Updated", second.DisplayName)
	}
}

// TestUserStore_UpsertByFirebaseUID_EmailCanonicalization asserts that the
// email is stored as lowercase and that the returned domain model reflects the
// canonicalized form.
func TestUserStore_UpsertByFirebaseUID_EmailCanonicalization(t *testing.T) {
	pool := newTestPool(t)
	store := NewUserStore(pool)
	ctx := context.Background()

	uid := "uid-canon-" + uuid.NewString()
	defer pool.Exec(ctx, `DELETE FROM users WHERE firebase_uid = $1`, uid)

	in := &models.User{FirebaseUID: uid, Email: "Mixed@CASE.COM"}
	out, err := store.UpsertByFirebaseUID(ctx, in)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if out.Email != "mixed@case.com" {
		t.Errorf("email = %q, want %q", out.Email, "mixed@case.com")
	}
}

// TestUserStore_UpsertByFirebaseUID_IdentityConflict asserts that attempting
// to claim an email already owned by a different firebase_uid returns
// ErrIdentityConflict and leaves the original row unchanged.
func TestUserStore_UpsertByFirebaseUID_IdentityConflict(t *testing.T) {
	pool := newTestPool(t)
	store := NewUserStore(pool)
	ctx := context.Background()

	sharedEmail := "conflict-" + uuid.NewString() + "@example.com"
	uidX := "uid-x-" + uuid.NewString()
	uidY := "uid-y-" + uuid.NewString()
	defer pool.Exec(ctx, `DELETE FROM users WHERE firebase_uid IN ($1, $2)`, uidX, uidY)

	// Seed uid-x with the shared email.
	first, err := store.UpsertByFirebaseUID(ctx, &models.User{FirebaseUID: uidX, Email: sharedEmail})
	if err != nil {
		t.Fatalf("seed uid-x: %v", err)
	}
	idX := first.ID

	// uid-y tries to claim the same email → conflict.
	_, err = store.UpsertByFirebaseUID(ctx, &models.User{FirebaseUID: uidY, Email: sharedEmail})
	if !errors.Is(err, models.ErrIdentityConflict) {
		t.Fatalf("expected ErrIdentityConflict, got %v", err)
	}

	// Original row must be unchanged.
	found, err := store.FindByID(ctx, idX)
	if err != nil {
		t.Fatalf("FindByID after conflict: %v", err)
	}
	if found.FirebaseUID != uidX {
		t.Errorf("original FirebaseUID changed: got %q, want %q", found.FirebaseUID, uidX)
	}
}

// TestUserStore_UpsertByFirebaseUID_CaseVariantConflict asserts the backend
// defense against email case-variant conflicts. Pre-seed (UID_X, "A@X.COM"),
// then attempt (UID_Y, "a@x.com") → ErrIdentityConflict.
func TestUserStore_UpsertByFirebaseUID_CaseVariantConflict(t *testing.T) {
	pool := newTestPool(t)
	store := NewUserStore(pool)
	ctx := context.Background()

	uidX := "uid-case-x-" + uuid.NewString()
	uidY := "uid-case-y-" + uuid.NewString()
	defer pool.Exec(ctx, `DELETE FROM users WHERE firebase_uid IN ($1, $2)`, uidX, uidY)

	// Seed with uppercase variant.
	outX, err := store.UpsertByFirebaseUID(ctx, &models.User{FirebaseUID: uidX, Email: "CASETEST@example.com"})
	if err != nil {
		t.Fatalf("seed uidX: %v", err)
	}
	// Canonicalized email must be lowercase.
	if outX.Email != "casetest@example.com" {
		t.Errorf("email = %q, want %q", outX.Email, "casetest@example.com")
	}
	idX := outX.ID

	// UID_Y tries lowercase variant (same canonical email).
	_, err = store.UpsertByFirebaseUID(ctx, &models.User{FirebaseUID: uidY, Email: "casetest@example.com"})
	if !errors.Is(err, models.ErrIdentityConflict) {
		t.Fatalf("expected ErrIdentityConflict for case-variant conflict, got %v", err)
	}

	// Original row unchanged.
	found, err := store.FindByID(ctx, idX)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.FirebaseUID != uidX {
		t.Errorf("original FirebaseUID = %q, want %q", found.FirebaseUID, uidX)
	}
}

// TestUserStore_FindByID_NotFound asserts that FindByID returns ErrNotFound
// for a UUID that does not exist in the database.
func TestUserStore_FindByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	store := NewUserStore(pool)
	ctx := context.Background()

	_, err := store.FindByID(ctx, uuid.New())
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
