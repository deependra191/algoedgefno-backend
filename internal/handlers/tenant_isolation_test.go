// Package handlers_test provides HTTP-boundary tenant isolation tests.
// These tests live in a separate _test package to allow importing internal/routes
// without creating an import cycle (routes imports handlers).
package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/handlers"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/routes"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mustOpenTenantIsolationPool opens a pgxpool.Pool and skips the test if
// TEST_DATABASE_URL is not set.
func mustOpenTenantIsolationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping tenant isolation test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// upsertIsolationUser creates a test user and registers cleanup.
func upsertIsolationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, firebaseUID, email string) *models.User {
	t.Helper()
	store := storage.NewUserStore(pool)
	user, err := store.UpsertByFirebaseUID(ctx, &models.User{
		FirebaseUID: firebaseUID,
		Email:       email,
	})
	if err != nil {
		t.Fatalf("upsert user %s: %v", firebaseUID, err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, user.ID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user
}

// TestTenantIsolation_AllSixRoutes is an HTTP-boundary isolation test that
// exercises protected routes and verifies that:
//   - requests without auth receive 401
//   - user A's valid token is accepted for tenant routes
//   - user B's valid token is accepted for tenant routes
//   - neither user can access a backtest they don't own (404)
//
// Skips when TEST_DATABASE_URL is not set.
func TestTenantIsolation_AllSixRoutes(t *testing.T) {
	pool := mustOpenTenantIsolationPool(t)
	ctx := context.Background()

	cfg := &config.Config{
		Env:            config.EnvTest,
		AppSecretToken: "test-secret",
		JWTSecret:      "test-jwt-secret-32-bytes-minimum!!",
	}

	userRepo := storage.NewUserStore(pool)
	tokenRepo := storage.NewRefreshTokenStore(pool)
	authSvc := services.NewAuthService(userRepo, tokenRepo, nil, cfg.JWTSecret, nil, cfg.Env)
	authHandler := handlers.NewAuthHandler(authSvc, cfg.Env)

	r := gin.New()
	routes.Register(r, pool, cfg, providers.NewRegistry(), authSvc, authHandler)

	// Seed two distinct users.
	userA := upsertIsolationUser(t, ctx, pool,
		"tenant-a-"+uuid.NewString(), "tenant-a-"+uuid.NewString()+"@test.com")
	userB := upsertIsolationUser(t, ctx, pool,
		"tenant-b-"+uuid.NewString(), "tenant-b-"+uuid.NewString()+"@test.com")

	// Mint tokens via DebugSession (works without Firebase verifier in test env).
	mintToken := func(t *testing.T, u *models.User) string {
		t.Helper()
		result, err := authSvc.DebugSession(ctx, u.FirebaseUID, u.Email, "")
		if err != nil {
			t.Fatalf("DebugSession for %s: %v", u.FirebaseUID, err)
		}
		t.Cleanup(func() {
			pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, u.ID)
		})
		return "Bearer " + result.AccessToken
	}

	tokenA := mintToken(t, userA)
	tokenB := mintToken(t, userB)

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/backtests"},
		{http.MethodGet, "/api/v1/strategies"},
	}

	t.Run("no_auth_header_401_on_tenant_routes", func(t *testing.T) {
		for _, rt := range protectedRoutes {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(rt.method, rt.path, nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without auth: got %d, want 401", rt.method, rt.path, w.Code)
			}
		}
	})

	t.Run("user_A_token_accepted", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/backtests", nil)
		req.Header.Set("Authorization", tokenA)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("user A /backtests: got %d, want 200; body: %s", w.Code, w.Body)
		}
	})

	t.Run("user_B_token_accepted", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/backtests", nil)
		req.Header.Set("Authorization", tokenB)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("user B /backtests: got %d, want 200; body: %s", w.Code, w.Body)
		}
	})

	t.Run("cross_tenant_backtest_detail_404", func(t *testing.T) {
		// A random UUID that neither user owns.
		fakeID := uuid.New()
		for _, token := range []string{tokenA, tokenB} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/backtests/"+fakeID.String(), nil)
			req.Header.Set("Authorization", token)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for non-owned backtest, got %d; body: %s", w.Code, w.Body)
			}
		}
	})

	t.Run("cross_tenant_backtest_trades_404", func(t *testing.T) {
		fakeID := uuid.New()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/backtests/"+fakeID.String()+"/trades", nil)
		req.Header.Set("Authorization", tokenA)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for non-owned trades, got %d", w.Code)
		}
	})
}
