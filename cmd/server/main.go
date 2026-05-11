package main

import (
	"log/slog"
	"os"

	"github.com/deependra191/algoedgefno-backend/internal/buildinfo"
	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
	"github.com/deependra191/algoedgefno-backend/internal/logging"
	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/providers/nse"
	"github.com/deependra191/algoedgefno-backend/internal/providers/vendor"
	"github.com/deependra191/algoedgefno-backend/internal/routes"
	"github.com/deependra191/algoedgefno-backend/internal/storage"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	// Configure structured logging before any further log output so that startup
	// failures also emit structured records. The returned logger carries env/version/commit
	// as base attributes on every record. It is also registered as slog.Default() so that
	// all package-level slog calls (e.g. slog.Info, slog.Error) share the same handler and
	// base attributes — the local variable is kept only to pass the same instance to
	// middleware.Logger, which accepts an injected logger for test isolation.
	logger := slog.New(logging.NewHandler(cfg.Env, os.Stderr)).With(
		"env", string(cfg.Env),
		"version", buildinfo.AppVersion,
		"commit", buildinfo.CommitSHA,
	)
	slog.SetDefault(logger)

	if err := cfg.ValidateStartupIdentity(); err != nil {
		// ValidateStartupIdentity returns errors built from non-secret values
		// (env name, db name, db user, db host). If that contract ever changes, review
		// this log call to ensure secrets are not included in the error string.
		slog.Error("startup validation failed", "error", err)
		os.Exit(1)
	}

	pool := database.Connect(cfg)
	defer pool.Close()

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)

	registry := providers.NewRegistry()
	registry.Register(nse.NewEODProvider(instrumentStore, candleStore,
		nse.WithUserAgent(cfg.NSEUserAgent),
		nse.WithAcceptHTML(cfg.NSEAcceptHTML),
	))
	registry.Register(vendor.NewStub())

	if cfg.Env == config.EnvProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Middleware order matters: Recovery first so that even a panic in RequestID or Logger
	// is caught and the client receives a 500 rather than a broken connection. RequestID
	// must run before Logger so the ID is available in the Gin context when Logger reads it.
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(cors.Default())

	routes.Register(r, pool, cfg, registry)

	slog.Info("server starting", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
