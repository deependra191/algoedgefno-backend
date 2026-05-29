package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWT claim keys (external token contract) and access-token lifetime.
const (
	// jwtSubClaim is the JWT standard subject claim key.
	jwtSubClaim = "sub"
	// jwtEnvClaim is the custom environment claim added to every minted token.
	// ValidateToken rejects tokens whose env claim does not match the service
	// environment, preventing cross-environment token reuse.
	jwtEnvClaim = "env"
	// jwtIssuedAtClaim is the JWT standard "issued at" claim key.
	jwtIssuedAtClaim = "iat"
	// jwtExpiryClaim is the JWT standard "expiration time" claim key.
	jwtExpiryClaim = "exp"

	// accessTTL is how long an access JWT remains valid.
	accessTTL = time.Hour
)

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

// mintAccessToken mints a signed HS256 JWT with sub=userID.String() and an
// env claim matching this service's environment.
func (s *AuthService) mintAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		jwtSubClaim:      userID.String(),
		jwtEnvClaim:      string(s.env),
		jwtIssuedAtClaim: now.Unix(),
		jwtExpiryClaim:   now.Add(accessTTL).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}
