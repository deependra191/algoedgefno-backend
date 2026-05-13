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

func Register(r *gin.Engine, pool *pgxpool.Pool, cfg *config.Config, registry *providers.Registry) {
	healthStore := storage.NewHealthStore(pool)
	healthSvc := services.NewHealthService(healthStore, cfg.Env)
	healthHandler := handlers.NewHealthHandler(healthSvc, cfg.Env)

	userRepo := storage.NewUserStore(pool)
	authSvc := services.NewAuthService(userRepo, cfg.JWTSecret)
	authHandler := handlers.NewAuthHandler(authSvc)

	backtestStore := storage.NewBacktestStore(pool)
	candleStore := storage.NewCandleStore(pool)
	instrumentStore := storage.NewInstrumentStore(pool)
	builtinRegistry := strategies.NewRegistry()

	strategySvc := services.NewStrategyService(builtinRegistry, backtestStore, candleStore)
	backtestSvc := services.NewBacktestService(backtestStore, builtinRegistry, candleStore, instrumentStore, engine.NewBacktester(), services.BacktestLimits{
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
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

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
