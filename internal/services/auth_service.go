package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// Log constants — per §6: event is a STRUCTURED ATTRIBUTE, not the message.
const (
	logMsgAuthAnomaly       = "auth anomaly"
	eventIdentityConflict   = "identity_conflict"
	reasonEmailDifferentUID = "email_collision_different_uid"

	// Structured log attribute keys (telemetry contract). The request-id key
	// is shared via models.LogAttrRequestID; these are local to the service.
	logAttrEvent  = "event"
	logAttrReason = "reason"
)

// SessionResult is returned by ExchangeFirebaseToken and DebugSession.
// It bundles the minted token pair with the upserted user.
type SessionResult struct {
	TokenPair
	User *models.User
}

// TokenPair holds the access and refresh tokens minted for a session.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// AuthService implements Firebase-based authentication, JWT minting,
// refresh-token rotation, and session logout. It is the sole emitter of
// identity-conflict log events (§6 single-emitter contract).
type AuthService struct {
	userRepo    models.UserRepository
	tokenRepo   models.RefreshTokenRepository
	fbVerifier  models.FirebaseVerifier // may be nil in dev/test
	jwtSecret   []byte
	allowedUIDs []string
	env         config.Environment
}

// NewAuthService constructs an AuthService. fbVerifier may be nil in dev/test
// when Firebase credentials are not configured; only ExchangeFirebaseToken is
// gated on a non-nil verifier.
func NewAuthService(
	userRepo models.UserRepository,
	tokenRepo models.RefreshTokenRepository,
	fbVerifier models.FirebaseVerifier,
	jwtSecret string,
	allowedUIDs []string,
	env config.Environment,
) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		tokenRepo:   tokenRepo,
		fbVerifier:  fbVerifier,
		jwtSecret:   []byte(jwtSecret),
		allowedUIDs: allowedUIDs,
		env:         env,
	}
}

// ExchangeFirebaseToken verifies the Firebase ID token, upserts the user row,
// and mints a new session (access + refresh tokens).
//
// Error mapping (performed by the handler):
//   - ErrFirebaseNotConfigured → 503
//   - ErrFirebaseTokenInvalid  → 401
//   - ErrFirebaseEmailUnverified → 403
//   - ErrAuthNotAllowed         → 403
//   - ErrIdentityConflict       → 409
//   - any other error           → 500
func (s *AuthService) ExchangeFirebaseToken(ctx context.Context, idToken string) (*SessionResult, error) {
	if s.fbVerifier == nil {
		return nil, models.ErrFirebaseNotConfigured
	}
	if len(idToken) > models.MaxFirebaseIDTokenLen {
		return nil, models.ErrInvalidRequest
	}
	claims, err := s.fbVerifier.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, models.ErrFirebaseTokenInvalid
	}
	if !claims.EmailVerified {
		return nil, models.ErrFirebaseEmailUnverified
	}
	if !s.allowlistAllows(claims.UID) {
		return nil, models.ErrAuthNotAllowed
	}

	user, err := s.userRepo.UpsertByFirebaseUID(ctx, &models.User{
		FirebaseUID: claims.UID,
		Email:       claims.Email,
		DisplayName: claims.DisplayName,
		PhotoURL:    claims.PhotoURL,
	})
	if err != nil {
		if errors.Is(err, models.ErrIdentityConflict) {
			// §6 single-emitter: service is the sole emitter of event=identity_conflict.
			s.logIdentityConflict(ctx, reasonEmailDifferentUID)
		}
		return nil, err
	}

	return s.mintSession(ctx, user)
}

// RefreshSession validates the raw refresh token, re-checks the allowlist,
// rotates the token pair, and mints a new access token.
//
// The switch statements carefully distinguish "this credential is not valid"
// (→ ErrRefreshTokenInvalid → 401) from "the database is misbehaving"
// (→ wrapped error → 500). Collapsing both into 401 would log the owner out
// on a transient DB blip AND mask an operational incident behind a
// routine-looking auth failure.
func (s *AuthService) RefreshSession(ctx context.Context, refreshToken string) (*TokenPair, error) {
	if !isValidRefreshTokenFormat(refreshToken) {
		return nil, models.ErrInvalidRequest
	}
	hash := sha256HexOf(refreshToken)

	userID, err := s.tokenRepo.LookupActiveUserID(ctx, hash)
	switch {
	case errors.Is(err, models.ErrNotFound):
		return nil, models.ErrRefreshTokenInvalid // unknown/expired/revoked hash
	case err != nil:
		return nil, fmt.Errorf("refresh: lookup active user: %w", err) // → 500
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	switch {
	case errors.Is(err, models.ErrNotFound):
		return nil, models.ErrRefreshTokenInvalid // user row deleted between issuance and refresh
	case err != nil:
		return nil, fmt.Errorf("refresh: find user: %w", err) // → 500
	}

	if !s.allowlistAllows(user.FirebaseUID) {
		return nil, models.ErrRefreshTokenInvalid // allowlist removal since issuance
	}

	newRaw, newHash, err := newRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("refresh: mint refresh token: %w", err) // → 500
	}

	_, err = s.tokenRepo.RotateRefreshToken(ctx, hash, &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: newHash,
		ExpiresAt: time.Now().UTC().Add(refreshTTL),
	})
	switch {
	case errors.Is(err, models.ErrNotFound):
		// Old hash was consumed by a concurrent rotation between
		// LookupActiveUserID and here — treat the credential as spent.
		return nil, models.ErrRefreshTokenInvalid
	case err != nil:
		return nil, fmt.Errorf("refresh: rotate: %w", err) // → 500
	}

	access, err := s.mintAccessToken(userID)
	if err != nil {
		return nil, fmt.Errorf("refresh: mint access token: %w", err) // → 500
	}
	return &TokenPair{AccessToken: access, RefreshToken: newRaw}, nil
}

// Logout revokes the refresh token identified by the raw token string.
// Idempotent — an absent or already-revoked token returns nil.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if !isValidRefreshTokenFormat(refreshToken) {
		return models.ErrInvalidRequest
	}
	return s.tokenRepo.RevokeByHash(ctx, sha256HexOf(refreshToken))
}

// DebugSession is available only in development and test environments. It
// upserts a user with the provided synthetic identity and mints a session
// without requiring a real Firebase ID token.
func (s *AuthService) DebugSession(ctx context.Context, uid, email, displayName string) (*SessionResult, error) {
	if s.env != config.EnvDevelopment && s.env != config.EnvTest {
		return nil, models.ErrNotAvailable
	}
	user, err := s.userRepo.UpsertByFirebaseUID(ctx, &models.User{
		FirebaseUID: uid,
		Email:       email,
		DisplayName: displayName,
	})
	if err != nil {
		return nil, err
	}
	return s.mintSession(ctx, user)
}

// mintSession mints a new token pair and persists the refresh token.
// If the user upsert succeeds but the refresh-token insert fails, a 500 is
// returned (deliberate non-atomic design — see §12 of the plan). Android
// retries; the upsert branch is idempotent.
func (s *AuthService) mintSession(ctx context.Context, user *models.User) (*SessionResult, error) {
	access, err := s.mintAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("mint session: access token: %w", err)
	}

	rawRefresh, hashRefresh, err := newRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("mint session: refresh token entropy: %w", err)
	}

	rt := &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: hashRefresh,
		ExpiresAt: time.Now().UTC().Add(refreshTTL),
	}
	if err := s.tokenRepo.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("mint session: persist refresh token: %w", err)
	}

	return &SessionResult{
		TokenPair: TokenPair{AccessToken: access, RefreshToken: rawRefresh},
		User:      user,
	}, nil
}

// allowlistAllows returns true when the UID is in the allowlist, or when the
// allowlist is empty in dev/test (allowlist disabled). In staging/prod an
// empty allowlist is rejected at startup by ValidateFirebaseAuthConfig, so
// reaching this function with an empty allowlist in those environments is not
// possible in normal operation.
func (s *AuthService) allowlistAllows(uid string) bool {
	if len(s.allowedUIDs) == 0 {
		// In dev/test, empty allowlist = allowlist disabled.
		return s.env == config.EnvDevelopment || s.env == config.EnvTest
	}
	for _, allowed := range s.allowedUIDs {
		if allowed == uid {
			return true
		}
	}
	return false
}

// logIdentityConflict emits a structured WARN record. It is the single
// authorised emitter of event=identity_conflict (§6). Only event, request_id,
// and reason are logged — no email, UID, or token values.
func (s *AuthService) logIdentityConflict(ctx context.Context, reason string) {
	slog.WarnContext(ctx, logMsgAuthAnomaly,
		slog.String(logAttrEvent, eventIdentityConflict),
		slog.String(models.LogAttrRequestID, models.RequestIDFrom(ctx)),
		slog.String(logAttrReason, reason),
	)
}
