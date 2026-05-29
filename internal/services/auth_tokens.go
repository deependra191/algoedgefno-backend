package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"time"
)

// Refresh-token format and lifetime.
const (
	// refreshTTL is how long a refresh token remains valid.
	refreshTTL = 30 * 24 * time.Hour

	// refreshTokenBytes is the number of cryptographically-random bytes drawn
	// for a refresh token before base64url encoding.
	refreshTokenBytes = 32
	// refreshTokenLen is the expected base64url-encoded length of a raw refresh
	// token (refreshTokenBytes bytes → 43 chars in base64 RawURL encoding).
	refreshTokenLen = 43
)

// refreshTokenCharset matches the base64url character set (no padding).
var refreshTokenCharset = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// sha256HexOf returns the SHA-256 hex digest of s. It is the single hash
// path used by issue, refresh, and logout — always hashes the base64url-
// encoded string (the exact bytes Android receives and re-sends). Hashing the
// pre-encoding random bytes would produce a different value, causing tokens to
// never match on lookup.
func sha256HexOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// newRefreshToken generates a cryptographically-random refreshTokenBytes-byte
// token, base64url-encodes it (43 chars, no padding), and returns both the raw
// encoded string (returned to Android) and its SHA-256 hex digest (stored
// server-side).
func newRefreshToken() (raw, hashHex string, err error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	raw = base64.RawURLEncoding.EncodeToString(buf) // 43 chars
	hashHex = sha256HexOf(raw)
	return
}

// isValidRefreshTokenFormat reports whether s is exactly refreshTokenLen
// characters long and contains only base64url characters (A-Za-z0-9_-).
// Malformed tokens are rejected before any DB lookup.
func isValidRefreshTokenFormat(s string) bool {
	return len(s) == refreshTokenLen && refreshTokenCharset.MatchString(s)
}
