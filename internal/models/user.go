// Package models defines domain objects that services operate on.
// Domain objects are distinct from:
//   - entities (DB scan targets in internal/entities/)
//   - response DTOs (handler-local structs in internal/handlers/*)
//
// Domain types must not carry `json:` tags and must not include
// DB-specific fields. Mappers convert between entity and domain forms.
package models

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User is the domain representation of a Firebase-backed user.
// PasswordHash and LastLoginAt are intentionally absent — they are storage
// concerns only (tracked in entities.User) and never surfaced to services or
// handlers.
type User struct {
	ID          uuid.UUID
	FirebaseUID string
	Email       string
	DisplayName string
	PhotoURL    string
	CreatedAt   time.Time
}

// UserRepository is the storage contract for user persistence.
type UserRepository interface {
	// UpsertByFirebaseUID inserts a new user row or updates the existing one
	// that matches firebase_uid. Returns the persisted domain user on success.
	// Returns ErrIdentityConflict when the email is already owned by a
	// different firebase_uid (23505 on users_email_key).
	UpsertByFirebaseUID(ctx context.Context, user *User) (*User, error)

	// FindByID returns the domain user for the given UUID primary key.
	// Returns ErrNotFound when no row exists with that id.
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
}
