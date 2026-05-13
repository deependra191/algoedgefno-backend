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
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	// Configure structured logging immediately after config loading so that startup
	// validation and runtime failures emit structured records. (config.Load() itself
	// may emit stdlib log output before this point.) The returned logger carries
	// env/version/commit as base attributes on every record. It is also registered as
	// slog.Default() so that all package-level slog calls share the same handler and
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
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		slog.Error("trusted proxy configuration failed", "error", err)
		os.Exit(1)
	}

	// Middleware order matters: Logger wraps Recovery so that when a handler panics,
	// Recovery catches it (sets 500) and returns normally, allowing Logger's post-c.Next()
	// code to run and emit a log record with the correct status and request_id.
	// RequestID runs first so the ID is in context before Logger reads it.
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(logger))
	r.Use(gin.Recovery())
	// CORS is intentionally disabled for v1 because the API is consumed only by
	// the native Android app. Add explicit browser/admin CORS in a separate PR
	// when there is an actual browser client.

	routes.Register(r, pool, cfg, registry)

	slog.Info("server starting", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
