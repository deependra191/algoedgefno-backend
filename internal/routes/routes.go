package routes

import (
	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/engine"
	"github.com/deependra191/algoedgefno-backend/internal/handlers"
	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
	"github.com/deependra191/algoedgefno-backend/internal/strategies"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Route-path constants — named per rule 17 (used in multiple places and in
// the rate-limiter key, so a reader cannot safely inline them).
const (
	routeAuthSession = "/api/v1/auth/session"
	routeAuthRefresh = "/api/v1/auth/refresh"
	routeAuthLogout  = "/api/v1/auth/logout"
)

// Body-cap constants (bytes) for auth endpoints.
const (
	// bodyCapSession is the pre-bind body limit for /auth/session and
	// /auth/debug-session. Firebase ID tokens can be up to ~4 KB; 8 KiB
	// gives comfortable headroom including the JSON wrapper.
	bodyCapSession = 8 * 1024

	// bodyCapRefreshLogout is the pre-bind body limit for /auth/refresh and
	// /auth/logout. Refresh tokens are 43 chars plus a small JSON wrapper.
	bodyCapRefreshLogout = 512
)

// Rate-limit (requests per minute) constants for auth endpoints.
const (
	rpmSession = 10
	rpmRefresh = 60
	rpmLogout  = 30
)

// Register wires all routes onto r. It creates handler dependencies from the
// provided pool and cfg, and uses authSvc and authHandler for the auth routes.
func Register(
	r *gin.Engine,
	pool *pgxpool.Pool,
	cfg *config.Config,
	registry *providers.Registry,
	authSvc *services.AuthService,
	authHandler *handlers.AuthHandler,
) {
	healthStore := storage.NewHealthStore(pool)
	healthSvc := services.NewHealthService(healthStore, cfg.Env)
	healthHandler := handlers.NewHealthHandler(healthSvc, cfg.Env)

	backtestStore := storage.NewBacktestStore(pool)
	candleStore := storage.NewCandleStore(pool)
	instrumentStore := storage.NewInstrumentStore(pool)
	builtinRegistry := strategies.NewRegistry()

	strategySvc := services.NewStrategyService(builtinRegistry, backtestStore, candleStore)
	backtestSvc := services.NewBacktestService(backtestStore, builtinRegistry, candleStore, instrumentStore, engine.NewBacktester(engine.NewCharges()), services.BacktestLimits{
		BacktestsEnabled: cfg.BacktestEnabled,
		MaxDays:          cfg.BacktestMaxDays,
		MaxCandles:       cfg.BacktestMaxCandles,
	})

	strategyHandler := handlers.NewStrategyHandler(strategySvc)
	backtestHandler := handlers.NewBacktestHandler(backtestSvc)

	r.GET("/health", healthHandler.Live)
	r.GET("/ready", healthHandler.Ready)
	r.GET("/version", healthHandler.Version)

	v1 := r.Group("/api/v1")
	{
		// Auth endpoints — public (no Auth middleware); protected by
		// RequestBodyLimit + RateLimit per-route.
		auth := v1.Group("/auth")
		{
			auth.POST("/session",
				middleware.RequestBodyLimit(bodyCapSession),
				middleware.RateLimit(routeAuthSession, rpmSession),
				authHandler.Session,
			)
			auth.POST("/refresh",
				middleware.RequestBodyLimit(bodyCapRefreshLogout),
				middleware.RateLimit(routeAuthRefresh, rpmRefresh),
				authHandler.Refresh,
			)
			auth.POST("/logout",
				middleware.RequestBodyLimit(bodyCapRefreshLogout),
				middleware.RateLimit(routeAuthLogout, rpmLogout),
				authHandler.Logout,
			)
			// Debug-session endpoint: only registered in development and test.
			// No rate limit — dev/test only, operator-facing.
			if cfg.Env == config.EnvDevelopment || cfg.Env == config.EnvTest {
				auth.POST("/debug-session",
					middleware.RequestBodyLimit(bodyCapSession),
					authHandler.DebugSession,
				)
			}
		}

		// Protected endpoints require a valid backend JWT (or the static
		// APP_SECRET_TOKEN on /config/app only).
		protected := v1.Group("")
		protected.Use(middleware.Auth(cfg.AppSecretToken, authSvc))
		{
			protected.GET("/config/app", handlers.AppConfig)

			protected.GET("/strategies", strategyHandler.List)
			protected.GET("/strategies/:id", strategyHandler.GetByID)

			protected.GET("/backtests", backtestHandler.List)
			protected.POST("/backtests", backtestHandler.Submit)
			protected.GET("/backtests/:id", backtestHandler.GetByID)
			protected.GET("/backtests/:id/trades", backtestHandler.GetTrades)
		}
	}

	_ = registry
}
