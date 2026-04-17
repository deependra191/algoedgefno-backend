// Package models defines domain objects that services operate on.
// Domain objects are distinct from:
//   - entities (DB scan targets in internal/entities/)
//   - response DTOs (handler-local structs in internal/handlers/*)
//
// Domain types must not carry `json:` tags and must not include
// DB-specific fields. Mappers convert between entity and domain forms.
package models

import (
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/entities"
)

// User is the domain representation of a user.
// The password hash lives only on the entity layer — it has no place
// in domain logic once authentication has completed.
type User struct {
	ID        uuid.UUID
	Email     string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FromUserEntity converts a DB entity into a domain User.
// The entity's PasswordHash is intentionally dropped.
func FromUserEntity(e *entities.User) *User {
	if e == nil {
		return nil
	}
	return &User{
		ID:        e.ID,
		Email:     e.Email,
		Name:      e.Name,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// ToEntity converts the domain User back into a DB entity.
// PasswordHash is left blank — callers must set it explicitly when
// persisting a new user.
func (u *User) ToEntity() *entities.User {
	if u == nil {
		return nil
	}
	return &entities.User{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
