package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newAuthRouter returns a minimal gin engine with the Auth middleware applied.
// It registers two routes: one on the config path (static token allowed) and
// one tenant path (all auth rejected in PR 1).
func newAuthRouter(appSecretToken string) *gin.Engine {
	r := gin.New()
	r.Use(Auth(appSecretToken))
	r.GET(appConfigPath, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/api/v1/backtests", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// doRequest fires a GET request against r with the given Authorization header value.
// Pass an empty string to omit the header.
func doRequest(r *gin.Engine, path, authHeader string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(w, req)
	return w
}

// assertAuthRejected asserts the canonical 401 response emitted by Auth.
func assertAuthRejected(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["error"] != errMissingOrInvalidAuthorizationHeader {
		t.Errorf("expected error %q, got %q", errMissingOrInvalidAuthorizationHeader, body["error"])
	}
}

// TestAuth_StaticTokenBlockedOnTenantEndpoints asserts that the static APP_SECRET_TOKEN
// is rejected on any path other than /api/v1/config/app (rows 1, 5).
func TestAuth_StaticTokenBlockedOnTenantEndpoints(t *testing.T) {
	const secret = "test-secret"
	r := newAuthRouter(secret)

	w := doRequest(r, "/api/v1/backtests", "Bearer "+secret)
	assertAuthRejected(t, w)
}

// TestAuth_StaticTokenAcceptedOnConfigApp asserts that a valid static token is
// accepted on the /api/v1/config/app path and the handler is invoked (row 2).
func TestAuth_StaticTokenAcceptedOnConfigApp(t *testing.T) {
	const secret = "test-secret"
	r := newAuthRouter(secret)

	w := doRequest(r, appConfigPath, "Bearer "+secret)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on config/app with valid static token, got %d", w.Code)
	}
}

// TestAuth_HMACJWTRejected asserts that a syntactically valid HS256 JWT is rejected
// on any path — the PR 1 middleware has no JWT path (row 3).
func TestAuth_HMACJWTRejected(t *testing.T) {
	const secret = "test-secret"
	const jwtSigningKey = "jwt-signing-key"
	r := newAuthRouter(secret)

	// Mint a valid HS256 JWT signed with a different key.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "11111111-1111-1111-1111-111111111111",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(jwtSigningKey))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	// Try on a tenant path.
	w := doRequest(r, "/api/v1/backtests", "Bearer "+tokenStr)
	assertAuthRejected(t, w)

	// Try on config/app — JWT is not the static token, so it must also be rejected.
	w = doRequest(r, appConfigPath, "Bearer "+tokenStr)
	assertAuthRejected(t, w)
}

// TestAuth_MissingAuthorizationHeader asserts that requests without an Authorization
// header receive 401 on all paths (row 4).
func TestAuth_MissingAuthorizationHeader(t *testing.T) {
	r := newAuthRouter("test-secret")

	for _, path := range []string{appConfigPath, "/api/v1/backtests"} {
		t.Run(fmt.Sprintf("path=%s", path), func(t *testing.T) {
			w := doRequest(r, path, "")
			assertAuthRejected(t, w)
		})
	}
}

// TestAuth_WrongTokenOnConfigPath asserts that the correct path check alone is not
// sufficient — the token must also match (row 5 sanity: both path + token must match).
func TestAuth_WrongTokenOnConfigPath(t *testing.T) {
	r := newAuthRouter("correct-secret")

	w := doRequest(r, appConfigPath, "Bearer wrong-secret")
	assertAuthRejected(t, w)
}
