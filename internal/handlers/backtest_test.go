package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// newBacktestListRouter returns a gin engine with a single /api/v1/backtests GET
// route that applies setKey then calls the real extractUserID helper.
// If identity is valid, it returns 200; if not, extractUserID aborts with 401.
func newBacktestListRouter(setKey func(c *gin.Context)) *gin.Engine {
	r := gin.New()
	r.GET("/api/v1/backtests", func(c *gin.Context) {
		setKey(c)
		_, ok := extractUserID(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// assertMissingUserIdentity asserts the response is 401 with the canonical error body.
func assertMissingUserIdentity(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["error"] != "missing user identity" {
		t.Errorf("expected error %q, got %q", "missing user identity", body["error"])
	}
}

// doGetJSON fires a GET request against r and returns the recorder.
func doGetJSON(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w
}

// TestBacktestHandler_MissingIdentity asserts that a missing UserIDKey causes 401 (row 7).
func TestBacktestHandler_MissingIdentity(t *testing.T) {
	r := newBacktestListRouter(func(_ *gin.Context) {
		// Key not set.
	})
	assertMissingUserIdentity(t, doGetJSON(r, "/api/v1/backtests"))
}

// TestBacktestHandler_StringIdentity asserts that a string value for UserIDKey causes 401 (row 8).
func TestBacktestHandler_StringIdentity(t *testing.T) {
	r := newBacktestListRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, "not-a-uuid")
	})
	assertMissingUserIdentity(t, doGetJSON(r, "/api/v1/backtests"))
}

// TestBacktestHandler_NilUUIDIdentity asserts that uuid.Nil as the UserIDKey value causes 401 (row 9).
func TestBacktestHandler_NilUUIDIdentity(t *testing.T) {
	r := newBacktestListRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, uuid.Nil)
	})
	assertMissingUserIdentity(t, doGetJSON(r, "/api/v1/backtests"))
}
