package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestEngine builds a gin.Engine with routes registered against a real pool if
// TEST_DATABASE_URL is set, or skips DB-backed tests when it is not.
// For route-existence tests (404 probes on removed paths) we only need the route
// table; storage is never invoked.  We use the real Register function with a live
// pool to ensure the route table produced by production code is what we test.
func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping routes test")
	}

	pool := mustOpenPool(t, dsn)

	cfg := &config.Config{
		AppSecretToken: "test-secret",
		Env:            config.EnvTest,
	}

	r := gin.New()
	Register(r, pool, cfg, providers.NewRegistry())
	return r
}

// TestRemovedRoutes_LoginAndRegisterReturn404 asserts that /api/v1/auth/login and
// /api/v1/auth/register are no longer registered after Phase D unregistered them (row 6).
func TestRemovedRoutes_LoginAndRegisterReturn404(t *testing.T) {
	r := newTestEngine(t)

	for _, path := range []string{
		"/api/v1/auth/login",
		"/api/v1/auth/register",
	} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for removed route %s, got %d", path, w.Code)
			}
		})
	}
}
