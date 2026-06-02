// Package handlers_test provides HTTP-boundary tenant isolation tests.
// These tests live in a separate _test package to allow importing internal/routes
// without creating an import cycle (routes imports handlers).
package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

// builtinMACrossoverSlug is the slug of the built-in "MA Crossover" strategy
// (internal/strategies). Used to attach a seeded backtest to a real strategy so
// the per-user lastBacktest lookup can be exercised at the HTTP boundary.
const builtinMACrossoverSlug = "ma_crossover"

type tenantFirebaseVerifier struct {
	claimsByToken map[string]*models.FirebaseClaims
}

func (v *tenantFirebaseVerifier) VerifyIDToken(_ context.Context, idToken string) (*models.FirebaseClaims, error) {
	claims, ok := v.claimsByToken[idToken]
	if !ok {
		return nil, errors.New("unknown firebase token")
	}
	return claims, nil
}

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

// seedCompletedBacktest inserts a COMPLETED backtest run (with >0 trades so it is
// list-visible) owned by userID and attached to the given built-in strategy slug.
// It registers cleanup and returns the created run.
func seedCompletedBacktest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, slug string) *models.BacktestRun {
	t.Helper()
	store := storage.NewBacktestStore(pool)

	underlying := models.UnderlyingNifty
	capital := 200000.0
	netPnl := 12345.0
	total := 5
	wins := 3
	losses := 2

	run := &models.BacktestRun{
		ID:              uuid.New(),
		UserID:          &userID,
		InstrumentToken: "NIFTY-FUT",
		FromTs:          time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ToTs:            time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
		CandleInterval:  models.CandleInterval1D,
		Status:          models.BacktestCompleted,
		StrategySlug:    &slug,
		Underlying:      &underlying,
		Capital:         &capital,
		NetPnl:          &netPnl,
		TotalTrades:     &total,
		WinCount:        &wins,
		LossCount:       &losses,
	}
	if err := store.Create(ctx, run); err != nil {
		t.Fatalf("create seeded backtest run: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM backtest_runs WHERE id = $1`, run.ID)
	})
	if err := store.UpdateResult(ctx, run); err != nil {
		t.Fatalf("complete seeded backtest run: %v", err)
	}
	return run
}

// TestTenantIsolation_AllSixRoutes is an HTTP-boundary isolation test covering
// the six protected tenant routes. It seeds a real backtest owned by user A and
// proves that user B cannot observe it through any read path:
//   - all six routes return 401 without auth
//   - both users' valid tokens are accepted on their own list routes
//   - B gets 404 on A's REAL backtest detail and trades (not a random UUID)
//   - A's /backtests list includes A's run; B's list excludes it
//   - A's strategy detail/list expose lastBacktest for A's run; B's show null
//   - POST /backtests is dispatched under the caller's own identity (no
//     spoofable user field), so a run can only be created for the JWT subject
//
// Skips when TEST_DATABASE_URL is not set.
func TestTenantIsolation_AllSixRoutes(t *testing.T) {
	pool := mustOpenTenantIsolationPool(t)
	ctx := context.Background()

	cfg := &config.Config{
		Env:                 config.EnvTest,
		AppSecretToken:      "test-secret",
		JWTSecret:           "test-jwt-secret-32-bytes-minimum!!",
		AllowedFirebaseUIDs: []string{"tenant-a-" + uuid.NewString(), "tenant-b-" + uuid.NewString()},
	}

	userRepo := storage.NewUserStore(pool)
	tokenRepo := storage.NewRefreshTokenStore(pool)
	const firebaseTokenA = "fake-firebase-token-a"
	const firebaseTokenB = "fake-firebase-token-b"
	claimsByToken := map[string]*models.FirebaseClaims{
		firebaseTokenA: {
			UID:           cfg.AllowedFirebaseUIDs[0],
			Email:         "tenant-a-" + uuid.NewString() + "@test.com",
			EmailVerified: true,
		},
		firebaseTokenB: {
			UID:           cfg.AllowedFirebaseUIDs[1],
			Email:         "tenant-b-" + uuid.NewString() + "@test.com",
			EmailVerified: true,
		},
	}
	authSvc := services.NewAuthService(
		userRepo, tokenRepo, &tenantFirebaseVerifier{claimsByToken: claimsByToken},
		cfg.JWTSecret, cfg.AllowedFirebaseUIDs, cfg.Env,
	)
	authHandler := handlers.NewAuthHandler(authSvc)

	r := gin.New()
	routes.Register(r, pool, cfg, providers.NewRegistry(), authSvc, authHandler)

	mintSession := func(t *testing.T, firebaseToken string) (*models.User, string) {
		t.Helper()
		result, err := authSvc.ExchangeFirebaseToken(ctx, firebaseToken)
		if err != nil {
			t.Fatalf("ExchangeFirebaseToken for %s: %v", firebaseToken, err)
		}
		t.Cleanup(func() {
			pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, result.User.ID)
			pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, result.User.ID)
		})
		return result.User, "Bearer " + result.AccessToken
	}

	userA, bearerTokenA := mintSession(t, firebaseTokenA)
	_, bearerTokenB := mintSession(t, firebaseTokenB)

	// A owns a completed backtest attached to a real built-in strategy slug.
	runA := seedCompletedBacktest(t, ctx, pool, userA.ID, builtinMACrossoverSlug)

	// do issues an authenticated request and returns the recorder.
	do := func(method, path, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		r.ServeHTTP(w, req)
		return w
	}

	allSixRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/strategies"},
		{http.MethodGet, "/api/v1/strategies/" + builtinMACrossoverSlug},
		{http.MethodGet, "/api/v1/backtests"},
		{http.MethodPost, "/api/v1/backtests"},
		{http.MethodGet, "/api/v1/backtests/" + runA.ID.String()},
		{http.MethodGet, "/api/v1/backtests/" + runA.ID.String() + "/trades"},
	}

	t.Run("no_auth_header_401_on_all_six_routes", func(t *testing.T) {
		for _, rt := range allSixRoutes {
			w := do(rt.method, rt.path, "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without auth: got %d, want 401", rt.method, rt.path, w.Code)
			}
		}
	})

	t.Run("owner_can_read_own_list", func(t *testing.T) {
		for _, token := range []struct {
			name string
			tok  string
		}{{"A", bearerTokenA}, {"B", bearerTokenB}} {
			w := do(http.MethodGet, "/api/v1/backtests", token.tok)
			if w.Code != http.StatusOK {
				t.Errorf("user %s /backtests: got %d, want 200; body: %s", token.name, w.Code, w.Body)
			}
		}
	})

	t.Run("owner_reads_own_backtest_detail_and_trades", func(t *testing.T) {
		if w := do(http.MethodGet, "/api/v1/backtests/"+runA.ID.String(), bearerTokenA); w.Code != http.StatusOK {
			t.Errorf("A own detail: got %d, want 200; body: %s", w.Code, w.Body)
		}
		if w := do(http.MethodGet, "/api/v1/backtests/"+runA.ID.String()+"/trades", bearerTokenA); w.Code != http.StatusOK {
			t.Errorf("A own trades: got %d, want 200; body: %s", w.Code, w.Body)
		}
	})

	t.Run("B_cannot_read_As_real_backtest_detail_404", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/backtests/"+runA.ID.String(), bearerTokenB)
		if w.Code != http.StatusNotFound {
			t.Errorf("B reading A's real backtest detail: got %d, want 404; body: %s", w.Code, w.Body)
		}
	})

	t.Run("B_cannot_read_As_real_backtest_trades_404", func(t *testing.T) {
		w := do(http.MethodGet, "/api/v1/backtests/"+runA.ID.String()+"/trades", bearerTokenB)
		if w.Code != http.StatusNotFound {
			t.Errorf("B reading A's real backtest trades: got %d, want 404; body: %s", w.Code, w.Body)
		}
	})

	t.Run("backtest_list_isolated_by_owner", func(t *testing.T) {
		aBody := do(http.MethodGet, "/api/v1/backtests", bearerTokenA).Body.String()
		if !strings.Contains(aBody, runA.ID.String()) {
			t.Errorf("A's /backtests list must include A's run %s; body: %s", runA.ID, aBody)
		}
		bBody := do(http.MethodGet, "/api/v1/backtests", bearerTokenB).Body.String()
		if strings.Contains(bBody, runA.ID.String()) {
			t.Errorf("B's /backtests list must NOT include A's run %s; body: %s", runA.ID, bBody)
		}
	})

	t.Run("strategy_detail_lastBacktest_isolated_by_owner", func(t *testing.T) {
		path := "/api/v1/strategies/" + builtinMACrossoverSlug
		aBody := do(http.MethodGet, path, bearerTokenA).Body.String()
		if !strings.Contains(aBody, runA.ID.String()) {
			t.Errorf("A's strategy detail must expose lastBacktest %s; body: %s", runA.ID, aBody)
		}
		bBody := do(http.MethodGet, path, bearerTokenB).Body.String()
		if strings.Contains(bBody, runA.ID.String()) {
			t.Errorf("B's strategy detail must NOT expose A's run %s; body: %s", runA.ID, bBody)
		}
		if !strings.Contains(bBody, `"lastBacktest":null`) {
			t.Errorf("B's strategy detail lastBacktest must be null; body: %s", bBody)
		}
	})

	t.Run("strategy_list_lastBacktest_isolated_by_owner", func(t *testing.T) {
		aBody := do(http.MethodGet, "/api/v1/strategies", bearerTokenA).Body.String()
		if !strings.Contains(aBody, runA.ID.String()) {
			t.Errorf("A's strategy list must expose lastBacktest %s; body: %s", runA.ID, aBody)
		}
		bBody := do(http.MethodGet, "/api/v1/strategies", bearerTokenB).Body.String()
		if strings.Contains(bBody, runA.ID.String()) {
			t.Errorf("B's strategy list must NOT expose A's run %s; body: %s", runA.ID, bBody)
		}
	})

	// POST ownership: the submit request body carries no user field
	// (backtestSubmitRequest is {strategyId, inputs}); the handler derives the
	// owner solely from the authenticated JWT and passes it to the service,
	// which sets it on the run before Create. So a caller can only ever create a
	// run for their own JWT subject. Here we assert the POST is dispatched under
	// A's identity (it passes auth and reaches the service — any non-auth status
	// is fine; a real 202 would require a market-data fixture whose instrument
	// resolution collides with other packages on the shared CI DB). The
	// persisted user_id == JWT-subject guarantee is proven deterministically at
	// the storage layer by TestBacktestStore_Create_PersistsUserID.
	t.Run("post_backtests_dispatched_under_callers_identity", func(t *testing.T) {
		w := do(http.MethodPost, "/api/v1/backtests", bearerTokenA)
		if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
			t.Errorf("POST /backtests as A must pass auth and dispatch to the service; got %d; body: %s", w.Code, w.Body)
		}
	})
}
