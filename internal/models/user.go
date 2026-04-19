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

// UserRepository is the storage contract for user persistence.
type UserRepository interface {
	// FindByEmail returns the domain user and their password hash.
	// The hash is returned separately so it never appears on models.User.
	// Returns ErrNotFound if no user exists with that email.
	FindByEmail(ctx context.Context, email string) (*User, string, error)
	// Create persists a new user with the given bcrypt password hash.
	Create(ctx context.Context, user *User, passwordHash string) error
}

// User is the domain representation of a user.
// PasswordHash is intentionally absent — it is returned separately by
// UserRepository.FindByEmail and never stored on the domain object.
type User struct {
	ID        uuid.UUID
	Email     string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
