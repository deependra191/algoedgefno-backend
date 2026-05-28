// Package firebaseauth implements models.FirebaseVerifier by wrapping the
// Firebase Admin SDK auth client. Only cmd/server should import this package;
// services, handlers, storage, middleware, and routes must not import it
// (enforced by arch-lint in Phase 6).
package firebaseauth

import (
	"context"

	firebaseadmin "firebase.google.com/go/v4/auth"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// Verifier wraps a Firebase Admin SDK auth client and implements
// models.FirebaseVerifier. It calls VerifyIDTokenAndCheckRevoked so that
// tokens that have been explicitly revoked via the Firebase Console or Admin
// SDK are rejected even within their otherwise valid lifetime.
type Verifier struct {
	client *firebaseadmin.Client
}

// NewVerifier constructs a Verifier backed by the given Firebase Auth client.
func NewVerifier(client *firebaseadmin.Client) *Verifier {
	return &Verifier{client: client}
}

// VerifyIDToken validates the Firebase ID token by checking its signature,
// expiry, project binding, and revocation status. It maps the Firebase token
// claims to models.FirebaseClaims. Any error from the SDK is returned as-is;
// the caller (AuthService) maps it to models.ErrFirebaseTokenInvalid.
func (v *Verifier) VerifyIDToken(ctx context.Context, idToken string) (*models.FirebaseClaims, error) {
	tok, err := v.client.VerifyIDTokenAndCheckRevoked(ctx, idToken)
	if err != nil {
		return nil, err
	}

	claims := &models.FirebaseClaims{
		UID:           tok.UID,
		EmailVerified: false,
	}

	if email, ok := tok.Claims["email"].(string); ok {
		claims.Email = email
	}
	if verified, ok := tok.Claims["email_verified"].(bool); ok {
		claims.EmailVerified = verified
	}
	if name, ok := tok.Claims["name"].(string); ok {
		claims.DisplayName = name
	}
	if photo, ok := tok.Claims["picture"].(string); ok {
		claims.PhotoURL = photo
	}

	return claims, nil
}
