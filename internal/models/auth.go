package models

import (
	"errors"

	"github.com/google/uuid"
)

// TokenValidator is the contract that middleware uses to validate a bearer
// token and resolve the authenticated user's UUID. Implemented by
// *services.AuthService so that middleware depends only on this interface,
// not on the concrete service type.
type TokenValidator interface {
	// ValidateToken parses the bearer token, verifies its signature and
	// environment claim, and returns the user's UUID. Returns uuid.Nil and a
	// non-nil error on any validation failure.
	ValidateToken(tokenStr string) (uuid.UUID, error)
}

// MaxFirebaseIDTokenLen is the maximum accepted byte length of a Firebase ID
// token. Firebase ID tokens are ~1–4 KB JWTs; the handler rejects longer
// bodies as 400 invalid_request and the service enforces the same cap as a
// belt-and-suspenders guard. Shared so both layers use one source of truth.
const MaxFirebaseIDTokenLen = 4096

var (
	// ErrIdentityConflict is returned when a firebase_uid→email mapping
	// conflicts with an existing email row owned by a different firebase_uid.
	// Storage raises this on 23505 for users_email_key; the handler maps it
	// to 409 identity_conflict.
	ErrIdentityConflict = errors.New("identity conflict")

	// ErrFirebaseNotConfigured is returned when the Firebase verifier is nil
	// (dev/test without credentials). Mapped to 503 firebase_not_configured.
	ErrFirebaseNotConfigured = errors.New("firebase not configured")

	// ErrFirebaseTokenInvalid is returned when Firebase rejects the ID token
	// (bad signature, expired, revoked, wrong project). Mapped to 401
	// firebase_token_invalid.
	ErrFirebaseTokenInvalid = errors.New("firebase token invalid")

	// ErrFirebaseEmailUnverified is returned when the Firebase user's
	// EmailVerified claim is false. Mapped to 403 firebase_email_unverified.
	ErrFirebaseEmailUnverified = errors.New("firebase email unverified")

	// ErrAuthNotAllowed is returned when the UID is not in the
	// ALLOWED_FIREBASE_UIDS allowlist (or the UID has been removed since
	// session issuance). Mapped to 403 auth_not_allowed.
	ErrAuthNotAllowed = errors.New("auth not allowed")

	// ErrRefreshTokenInvalid is returned when a refresh token is not found,
	// expired, revoked, owned by a deleted user, or consumed by a concurrent
	// rotation. NEVER used for transport- or DB-level failures — those
	// propagate as wrapped errors and map to 500 internal.
	// Mapped to 401 refresh_token_invalid.
	ErrRefreshTokenInvalid = errors.New("refresh token invalid")

	// ErrInvalidRequest is the bound-violation sentinel surfaced to handlers
	// as 400 invalid_request (oversized token body, malformed refresh-token
	// format, etc.).
	ErrInvalidRequest = errors.New("invalid request")
)
