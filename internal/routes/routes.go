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

// Route-path constants — named per rule 17 for API contract paths and
// rate-limiter keys.
const (
	routeAuthSession = "/api/v1/auth/session"
	routeAuthRefresh = "/api/v1/auth/refresh"
	routeAuthLogout  = "/api/v1/auth/logout"
	routeConfigApp   = "/config/app"
)

// Body-cap constants (bytes) for auth endpoints.
const (
	// bodyCapSession is the pre-bind body limit for /auth/session. Firebase ID
	// tokens can be up to ~4 KB; 8 KiB gives comfortable headroom including the
	// JSON wrapper.
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
		}

		v1.GET(routeConfigApp, handlers.AppConfig)

		// Protected endpoints require a valid backend JWT.
		protected := v1.Group("")
		protected.Use(middleware.Auth(authSvc))
		{
			// Tenant endpoints additionally require a valid user identity in
			// context. RequireUserIdentity runs after Auth and enforces the
			// route-level invariant so handlers can read the owner via
			// mustUserID without re-validating.
			tenant := protected.Group("")
			tenant.Use(middleware.RequireUserIdentity())
			{
				tenant.GET("/strategies", strategyHandler.List)
				tenant.GET("/strategies/:id", strategyHandler.GetByID)

				tenant.GET("/backtests", backtestHandler.List)
				tenant.POST("/backtests", backtestHandler.Submit)
				tenant.GET("/backtests/:id", backtestHandler.GetByID)
				tenant.GET("/backtests/:id/trades", backtestHandler.GetTrades)
			}
		}
	}

	_ = registry
}
