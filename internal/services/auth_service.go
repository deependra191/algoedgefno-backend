package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// JWT claim and timing constants.
const (
	// jwtSubClaim is the JWT standard subject claim key.
	jwtSubClaim = "sub"
	// jwtEnvClaim is the custom environment claim added to every minted token.
	// ValidateToken rejects tokens whose env claim does not match the service
	// environment, preventing cross-environment token reuse.
	jwtEnvClaim = "env"

	// accessTTL is how long an access JWT remains valid.
	accessTTL = time.Hour
	// refreshTTL is how long a refresh token remains valid.
	refreshTTL = 30 * 24 * time.Hour

	// refreshTokenLen is the expected base64url-encoded length of a raw
	// refresh token (32 random bytes → 43 chars in base64 RawURL encoding).
	refreshTokenLen = 43
)

// Log constants — per §6: event is a STRUCTURED ATTRIBUTE, not the message.
const (
	logMsgAuthAnomaly       = "auth anomaly"
	eventIdentityConflict   = "identity_conflict"
	reasonEmailDifferentUID = "email_collision_different_uid"
)

// refreshTokenCharset matches the base64url character set (no padding).
var refreshTokenCharset = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

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

// ValidateToken implements models.TokenValidator. It parses the HS256 bearer
// token, verifies the env claim matches this service's environment, and
// returns the user UUID from the sub claim. Returns uuid.Nil and an error on
// any failure.
func (s *AuthService) ValidateToken(tokenStr string) (uuid.UUID, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !tok.Valid {
		return uuid.Nil, errors.New("invalid token")
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, errors.New("invalid token claims")
	}

	// Reject tokens issued for a different environment (cross-env reuse).
	envClaim, _ := claims[jwtEnvClaim].(string)
	if envClaim != string(s.env) {
		return uuid.Nil, errors.New("token environment mismatch")
	}

	sub, ok := claims[jwtSubClaim].(string)
	if !ok {
		return uuid.Nil, errors.New("invalid token subject")
	}
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse token subject: %w", err)
	}
	return id, nil
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
	if len(idToken) > 4096 {
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

// mintAccessToken mints a signed HS256 JWT with sub=userID.String() and an
// env claim matching this service's environment.
func (s *AuthService) mintAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		jwtSubClaim: userID.String(),
		jwtEnvClaim: string(s.env),
		"iat":       now.Unix(),
		"exp":       now.Add(accessTTL).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}

// allowlistAllows returns true when the UID is in the allowlist, or when the
// allowlist is empty in dev/test (allowlist disabled). In staging/prod an
// empty allowlist is rejected at startup by ValidateServerConfig, so reaching
// this function with an empty allowlist in those environments is not possible
// in normal operation.
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
		slog.String("event", eventIdentityConflict),
		slog.String(models.LogAttrRequestID, models.RequestIDFrom(ctx)),
		slog.String("reason", reason),
	)
}

// sha256HexOf returns the SHA-256 hex digest of s. It is the single hash
// path used by issue, refresh, and logout — always hashes the base64url-
// encoded string (the exact bytes Android receives and re-sends). Hashing the
// pre-encoding random bytes would produce a different value, causing tokens to
// never match on lookup.
func sha256HexOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// newRefreshToken generates a cryptographically-random 32-byte token,
// base64url-encodes it (43 chars, no padding), and returns both the raw
// encoded string (returned to Android) and its SHA-256 hex digest (stored
// server-side).
func newRefreshToken() (raw, hashHex string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	raw = base64.RawURLEncoding.EncodeToString(buf) // 43 chars
	hashHex = sha256HexOf(raw)
	return
}

// isValidRefreshTokenFormat reports whether s is exactly 43 characters long
// and contains only base64url characters (A-Za-z0-9_-). Malformed tokens are
// rejected before any DB lookup.
func isValidRefreshTokenFormat(s string) bool {
	return len(s) == refreshTokenLen && refreshTokenCharset.MatchString(s)
}
