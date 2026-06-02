package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const testTenantPath = "/api/v1/backtests"

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
func newAuthRouter() *gin.Engine {
	return newAuthRouterWithValidator(alwaysRejectValidator{})
}

// newAuthRouterWithValidator builds a router with an injectable TokenValidator.
func newAuthRouterWithValidator(v models.TokenValidator) *gin.Engine {
	r := gin.New()
	r.Use(Auth(v))
	r.GET(testTenantPath, func(c *gin.Context) {
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

// TestAuth_RejectedTokenOnTenantEndpoint asserts that any bearer token rejected
// by the validator is rejected by the middleware.
func TestAuth_RejectedTokenOnTenantEndpoint(t *testing.T) {
	r := newAuthRouter()

	w := doRequest(r, testTenantPath, "Bearer rejected-token")
	assertTokenRejected(t, w)
}

// TestAuth_ValidJWTAcceptedOnTenantEndpoint asserts that a token accepted by
// the validator is allowed through on tenant paths.
func TestAuth_ValidJWTAcceptedOnTenantEndpoint(t *testing.T) {
	fixedUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	r := newAuthRouterWithValidator(alwaysAcceptValidator{uid: fixedUID})

	w := doRequest(r, testTenantPath, "Bearer some-jwt-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid JWT on tenant path, got %d; body: %s", w.Code, w.Body)
	}
}

// TestAuth_HMACJWTPassedToValidator asserts that a syntactically valid HS256 JWT
// is passed to the TokenValidator. With alwaysRejectValidator the result is 401
// — what matters is that the middleware delegates to the validator rather than
// silently accepting the JWT.
func TestAuth_HMACJWTPassedToValidator(t *testing.T) {
	const jwtSigningKey = "jwt-signing-key"
	r := newAuthRouter()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "11111111-1111-1111-1111-111111111111",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(jwtSigningKey))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	w := doRequest(r, testTenantPath, "Bearer "+tokenStr)
	assertTokenRejected(t, w)
}

// TestAuth_MissingAuthorizationHeader asserts that requests without an
// Authorization header receive 401.
func TestAuth_MissingAuthorizationHeader(t *testing.T) {
	r := newAuthRouter()

	w := doRequest(r, testTenantPath, "")
	assertAuthRejected(t, w)
}

// TestAuth_UserIDKeySetOnSuccess asserts that a valid JWT causes the middleware
// to set models.UserIDKey in the Gin context as a uuid.UUID.
func TestAuth_UserIDKeySetOnSuccess(t *testing.T) {
	fixedUID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	r := gin.New()
	r.Use(Auth(alwaysAcceptValidator{uid: fixedUID}))
	r.GET(testTenantPath, func(c *gin.Context) {
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

	w := doRequest(r, testTenantPath, "Bearer some-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
