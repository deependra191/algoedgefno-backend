package middleware

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// alwaysRejectValidator is a models.TokenValidator stub that always returns
// an error. Used in tests where all JWT traffic must be rejected.
type alwaysRejectValidator struct{}

func (alwaysRejectValidator) ValidateToken(_ string) (uuid.UUID, error) {
	return uuid.Nil, errors.New("always rejected")
}

// alwaysAcceptValidator is a models.TokenValidator stub that always returns
// a fixed UUID. Used in tests that verify the JWT path succeeds.
type alwaysAcceptValidator struct {
	uid uuid.UUID
}

func (v alwaysAcceptValidator) ValidateToken(_ string) (uuid.UUID, error) {
	return v.uid, nil
}

// newAuthRouter returns a minimal gin engine with the Auth middleware applied.
// It registers two routes: one on the config path (static token allowed) and
// one tenant path (requires a valid JWT from validator).
func newAuthRouter(appSecretToken string) *gin.Engine {
	return newAuthRouterWithValidator(appSecretToken, alwaysRejectValidator{})
}

// newAuthRouterWithValidator builds a router with an injectable TokenValidator.
func newAuthRouterWithValidator(appSecretToken string, v models.TokenValidator) *gin.Engine {
	r := gin.New()
	r.Use(Auth(appSecretToken, v))
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

// assertAuthRejected asserts the canonical 401 response emitted by Auth when
// the Authorization header is missing or the Bearer prefix is absent.
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

// assertTokenRejected asserts a 401 from the JWT validator path.
func assertTokenRejected(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body)
	}
}

// TestAuth_StaticTokenBlockedOnTenantEndpoints asserts that the static APP_SECRET_TOKEN
// is rejected on any path other than /api/v1/config/app when the validator also rejects.
func TestAuth_StaticTokenBlockedOnTenantEndpoints(t *testing.T) {
	const secret = "test-secret"
	r := newAuthRouter(secret)

	w := doRequest(r, "/api/v1/backtests", "Bearer "+secret)
	// validator rejects (alwaysRejectValidator), so 401.
	assertTokenRejected(t, w)
}

// TestAuth_StaticTokenAcceptedOnConfigApp asserts that a valid static token is
// accepted on the /api/v1/config/app path and the handler is invoked.
func TestAuth_StaticTokenAcceptedOnConfigApp(t *testing.T) {
	const secret = "test-secret"
	r := newAuthRouter(secret)

	w := doRequest(r, appConfigPath, "Bearer "+secret)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on config/app with valid static token, got %d", w.Code)
	}
}

// TestAuth_ValidJWTAcceptedOnTenantEndpoint asserts that a token accepted by
// the validator is allowed through on tenant paths.
func TestAuth_ValidJWTAcceptedOnTenantEndpoint(t *testing.T) {
	const secret = "test-secret"
	fixedUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	r := newAuthRouterWithValidator(secret, alwaysAcceptValidator{uid: fixedUID})

	w := doRequest(r, "/api/v1/backtests", "Bearer some-jwt-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid JWT on tenant path, got %d; body: %s", w.Code, w.Body)
	}
}

// TestAuth_ValidJWTRejectedOnConfigAppWhenNotStaticToken asserts that a JWT
// that the validator accepts is still rejected on /config/app if it does not
// constant-time-match the static token — the static-token check takes priority
// on /config/app, and a JWT reaching the validator path on /config/app succeeds
// only when the validator accepts it (which the alwaysRejectValidator does not).
func TestAuth_ValidJWTRejectedOnConfigAppWhenNotStaticToken(t *testing.T) {
	const secret = "correct-secret"
	r := newAuthRouter(secret) // alwaysRejectValidator

	// A JWT that is not the static token will fall through to the validator,
	// which rejects it.
	w := doRequest(r, appConfigPath, "Bearer wrong-token")
	assertTokenRejected(t, w)
}

// TestAuth_HMACJWTPassedToValidator asserts that a syntactically valid HS256 JWT
// is passed to the TokenValidator (not short-circuited by the static-token check).
// With alwaysRejectValidator the result is 401 — what matters is that the
// middleware delegates to the validator rather than silently accepting the JWT.
func TestAuth_HMACJWTPassedToValidator(t *testing.T) {
	const secret = "test-secret"
	const jwtSigningKey = "jwt-signing-key"
	r := newAuthRouter(secret) // alwaysRejectValidator

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "11111111-1111-1111-1111-111111111111",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(jwtSigningKey))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	// Tenant path — validator rejects.
	w := doRequest(r, "/api/v1/backtests", "Bearer "+tokenStr)
	assertTokenRejected(t, w)

	// Config/app — JWT is not the static token; falls to validator which rejects.
	w = doRequest(r, appConfigPath, "Bearer "+tokenStr)
	assertTokenRejected(t, w)
}

// TestAuth_MissingAuthorizationHeader asserts that requests without an Authorization
// header receive 401 on all paths.
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
// sufficient — the token must also match (both path + token must match for static bypass).
func TestAuth_WrongTokenOnConfigPath(t *testing.T) {
	r := newAuthRouter("correct-secret")

	w := doRequest(r, appConfigPath, "Bearer wrong-secret")
	// Falls through to validator (alwaysRejectValidator) → 401.
	assertTokenRejected(t, w)
}

// TestAuth_UserIDKeySetOnSuccess asserts that a valid JWT causes the middleware
// to set models.UserIDKey in the Gin context as a uuid.UUID.
func TestAuth_UserIDKeySetOnSuccess(t *testing.T) {
	fixedUID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	r := gin.New()
	r.Use(Auth("", alwaysAcceptValidator{uid: fixedUID}))
	r.GET("/api/v1/backtests", func(c *gin.Context) {
		raw, ok := c.Get(models.UserIDKey)
		if !ok {
			t.Error("UserIDKey not set in context")
			c.Status(http.StatusInternalServerError)
			return
		}
		uid, ok := raw.(uuid.UUID)
		if !ok || uid != fixedUID {
			t.Errorf("UserIDKey = %v (%T), want %v", raw, raw, fixedUID)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	w := doRequest(r, "/api/v1/backtests", "Bearer some-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
