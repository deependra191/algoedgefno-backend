package models

import "context"

// FirebaseVerifier verifies a Firebase ID token and extracts the identity
// claims. Implemented by internal/firebaseauth; may be nil in dev/test
// environments where Firebase credentials are not configured.
type FirebaseVerifier interface {
	// VerifyIDToken validates the Firebase ID token and returns the claims.
	// Returns a non-nil error if the token is invalid, expired, revoked, or
	// issued for a different project.
	VerifyIDToken(ctx context.Context, idToken string) (*FirebaseClaims, error)
}

// FirebaseClaims contains the identity fields extracted from a verified
// Firebase ID token. Only the fields consumed by the auth service are
// included here.
type FirebaseClaims struct {
	UID           string
	Email         string
	DisplayName   string
	PhotoURL      string
	EmailVerified bool
}
