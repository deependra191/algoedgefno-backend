package handlers

import (
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/services"
)

// Auth error-response codes — the value of the {"error": "<code>"} body for
// every auth endpoint. These are the Android error contract; Android maps them
// to user-facing messages, so they must stay stable and centrally defined.
const (
	errCodeInvalidRequest          = "invalid_request"
	errCodeFirebaseNotConfigured   = "firebase_not_configured"
	errCodeFirebaseTokenInvalid    = "firebase_token_invalid"
	errCodeFirebaseEmailUnverified = "firebase_email_unverified"
	errCodeAuthNotAllowed          = "auth_not_allowed"
	errCodeIdentityConflict        = "identity_conflict"
	errCodeRefreshTokenInvalid     = "refresh_token_invalid"
	errCodeNotAvailable            = "not_available"
	errCodeInternal                = "internal"
)

// authResponse is the wire shape returned by POST /api/v1/auth/session and
// POST /api/v1/auth/debug-session. JSON keys are the Android contract.
type authResponse struct {
	AccessToken  string       `json:"accessToken"`
	RefreshToken string       `json:"refreshToken"`
	User         userResponse `json:"user"`
}

// refreshResponse is the wire shape returned by POST /api/v1/auth/refresh.
// It intentionally omits the user object — the client already has it from
// the session exchange.
type refreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// userResponse is the wire shape of a user returned to Android.
// It omits credential fields (no passwordHash, no name, no updatedAt,
// no lastLoginAt) so a forgotten field cannot accidentally leak.
type userResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	PhotoURL    string    `json:"photoUrl,omitempty"`
}

// toAuthResponse maps a SessionResult to the authResponse wire DTO.
func toAuthResponse(r *services.SessionResult) authResponse {
	return authResponse{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		User: userResponse{
			ID:          r.User.ID,
			Email:       r.User.Email,
			DisplayName: r.User.DisplayName,
			PhotoURL:    r.User.PhotoURL,
		},
	}
}
