package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/handlers"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestEngine builds a gin.Engine with routes registered against a real pool if
// TEST_DATABASE_URL is set, or skips DB-backed tests when it is not.
// For route-existence tests (404 probes on removed paths) we only need the route
// table; storage is never invoked. We use the real Register function with a live
// pool to ensure the route table produced by production code is what we test.
func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping routes test")
	}

	pool := mustOpenPool(t, dsn)

	cfg := &config.Config{
		Env:       config.EnvTest,
		JWTSecret: "test-jwt-secret",
	}

	userRepo := storage.NewUserStore(pool)
	tokenRepo := storage.NewRefreshTokenStore(pool)
	authSvc := services.NewAuthService(userRepo, tokenRepo, nil, cfg.JWTSecret, nil, cfg.Env)
	authHandler := handlers.NewAuthHandler(authSvc)

	r := gin.New()
	Register(r, pool, cfg, providers.NewRegistry(), authSvc, authHandler)
	return r
}

func newRouteTableOnlyEngine(env config.Environment) *gin.Engine {
	cfg := &config.Config{
		Env:       env,
		JWTSecret: "test-jwt-secret",
	}
	authSvc := services.NewAuthService(nil, nil, nil, cfg.JWTSecret, nil, cfg.Env)
	authHandler := handlers.NewAuthHandler(authSvc)

	r := gin.New()
	Register(r, nil, cfg, providers.NewRegistry(), authSvc, authHandler)
	return r
}

func TestConfigAppRouteIsPublic(t *testing.T) {
	r := newRouteTableOnlyEngine(config.EnvTest)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/app", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for public config/app route, got %d", w.Code)
	}
}

// TestRemovedRoutes_LoginAndRegisterReturn404 asserts that /api/v1/auth/login and
// /api/v1/auth/register are no longer registered after password auth was removed.
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

func TestRemovedRoutes_DebugSessionReturn404InAllEnvironments(t *testing.T) {
	for _, env := range []config.Environment{
		config.EnvDevelopment,
		config.EnvTest,
		config.EnvStaging,
		config.EnvProduction,
	} {
		t.Run(string(env), func(t *testing.T) {
			r := newRouteTableOnlyEngine(env)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/debug-session", nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for removed debug-session route in %s, got %d", env, w.Code)
			}
		})
	}
}

// TestTenantRoutesAreGuarded asserts every tenant route produced by the real
// Register wiring rejects an unauthenticated request with 401. It locks the
// route table so a future refactor that registers a strategy/backtest endpoint
// outside the auth-guarded tenant subgroup fails here instead of reaching a
// handler that would panic in mustUserID. The 401 is emitted by Auth (which
// runs ahead of RequireUserIdentity in the same group), so no DB is touched and
// the test runs without TEST_DATABASE_URL.
func TestTenantRoutesAreGuarded(t *testing.T) {
	r := newRouteTableOnlyEngine(config.EnvTest)

	tenantRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/strategies"},
		{http.MethodGet, "/api/v1/strategies/ma_crossover"},
		{http.MethodGet, "/api/v1/backtests"},
		{http.MethodPost, "/api/v1/backtests"},
		{http.MethodGet, "/api/v1/backtests/11111111-1111-1111-1111-111111111111"},
		{http.MethodGet, "/api/v1/backtests/11111111-1111-1111-1111-111111111111/trades"},
	}

	for _, rt := range tenantRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(rt.method, rt.path, nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for unauthenticated %s %s, got %d", rt.method, rt.path, w.Code)
			}
		})
	}
}
