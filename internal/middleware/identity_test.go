package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// newRequireIdentityRouter builds a router that optionally runs setKey to seed
// the context, then RequireUserIdentity, then a 200 handler. A nil setKey
// leaves models.UserIDKey unset, exercising the missing-identity path.
func newRequireIdentityRouter(setKey gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	chain := []gin.HandlerFunc{}
	if setKey != nil {
		chain = append(chain, setKey)
	}
	chain = append(chain, RequireUserIdentity(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET(testTenantPath, chain...)
	return r
}

// assertMissingUserIdentity asserts the canonical 401 emitted by
// RequireUserIdentity when the context identity is absent, mistyped, or nil.
func assertMissingUserIdentity(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["error"] != errMissingUserIdentity {
		t.Errorf("expected error %q, got %q", errMissingUserIdentity, body["error"])
	}
}

// TestRequireUserIdentity_MissingKey asserts that an unset UserIDKey causes 401.
func TestRequireUserIdentity_MissingKey(t *testing.T) {
	r := newRequireIdentityRouter(nil)
	assertMissingUserIdentity(t, doRequest(r, testTenantPath, ""))
}

// TestRequireUserIdentity_WrongType asserts that a non-uuid value causes 401.
func TestRequireUserIdentity_WrongType(t *testing.T) {
	r := newRequireIdentityRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, "not-a-uuid")
	})
	assertMissingUserIdentity(t, doRequest(r, testTenantPath, ""))
}

// TestRequireUserIdentity_NilUUID asserts that uuid.Nil causes 401.
func TestRequireUserIdentity_NilUUID(t *testing.T) {
	r := newRequireIdentityRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, uuid.Nil)
	})
	assertMissingUserIdentity(t, doRequest(r, testTenantPath, ""))
}

// TestRequireUserIdentity_ValidUUID asserts that a valid UUID lets the request
// proceed to the handler.
func TestRequireUserIdentity_ValidUUID(t *testing.T) {
	r := newRequireIdentityRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, uuid.New())
	})
	w := doRequest(r, testTenantPath, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid identity, got %d; body: %s", w.Code, w.Body)
	}
}

// TestRequireUserIdentity_AfterAuthValidJWT asserts the ordering invariant: a
// valid JWT flows through Auth (which sets the identity) and then through
// RequireUserIdentity to the handler.
func TestRequireUserIdentity_AfterAuthValidJWT(t *testing.T) {
	fixedUID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	r := gin.New()
	r.GET(testTenantPath,
		Auth(alwaysAcceptValidator{uid: fixedUID}),
		RequireUserIdentity(),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) },
	)

	w := doRequest(r, testTenantPath, "Bearer some-jwt-token")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 through Auth+RequireUserIdentity, got %d; body: %s", w.Code, w.Body)
	}
}

// TestRequireUserIdentity_RejectsTamperedIdentityAfterAuth asserts that
// RequireUserIdentity is a backstop: even when Auth has set a valid identity,
// a later middleware that corrupts UserIDKey is caught before the handler runs.
func TestRequireUserIdentity_RejectsTamperedIdentityAfterAuth(t *testing.T) {
	fixedUID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	r := gin.New()
	r.GET(testTenantPath,
		Auth(alwaysAcceptValidator{uid: fixedUID}),
		func(c *gin.Context) { c.Set(models.UserIDKey, "tampered") },
		RequireUserIdentity(),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) },
	)

	assertMissingUserIdentity(t, doRequest(r, testTenantPath, "Bearer some-jwt-token"))
}
