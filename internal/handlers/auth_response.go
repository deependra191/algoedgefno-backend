package handlers

import (
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// userResponse is the wire shape for a user returned to Android.
// It intentionally omits any credential fields. JSON keys match the
// prior shape of models.User to keep the Android contract stable.
type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type authResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

// toUserResponse maps the domain user to the wire DTO.
func toUserResponse(u *models.User) userResponse {
	if u == nil {
		return userResponse{}
	}
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
