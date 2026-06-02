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
