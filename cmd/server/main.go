package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"

	"github.com/deependra191/algoedgefno-backend/internal/buildinfo"
	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/database"
	"github.com/deependra191/algoedgefno-backend/internal/firebaseauth"
	"github.com/deependra191/algoedgefno-backend/internal/handlers"
	"github.com/deependra191/algoedgefno-backend/internal/logging"
	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/providers"
	"github.com/deependra191/algoedgefno-backend/internal/providers/nse"
	"github.com/deependra191/algoedgefno-backend/internal/providers/vendor"
	"github.com/deependra191/algoedgefno-backend/internal/routes"
	"github.com/deependra191/algoedgefno-backend/internal/services"
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

	if err := config.ValidateServerConfig(cfg); err != nil {
		slog.Error("server config validation failed", "error", err)
		os.Exit(1)
	}

	// cleanup-expired-refresh-tokens subcommand — build a minimal pool,
	// delete expired rows, log the count, then exit.
	if len(os.Args) > 1 && os.Args[1] == "cleanup-expired-refresh-tokens" {
		pool := database.Connect(cfg)
		defer pool.Close()
		tokenRepo := storage.NewRefreshTokenStore(pool)
		n, err := tokenRepo.DeleteExpired(context.Background())
		if err != nil {
			slog.Error("cleanup expired refresh tokens failed", "error", err)
			os.Exit(1)
		}
		slog.Info("cleanup expired refresh tokens complete", "rows_deleted", n)
		return
	}

	ctx := context.Background()

	// Build the Firebase verifier. When FIREBASE_CREDENTIALS_FILE is unset
	// (dev/test without credentials), verifier stays nil and only
	// ExchangeFirebaseToken is gated — Refresh/Logout work without it.
	var verifier models.FirebaseVerifier
	if cfg.FirebaseCredentialsFile != "" {
		fbApp, err := firebase.NewApp(ctx,
			&firebase.Config{ProjectID: cfg.FirebaseProjectID},
			option.WithCredentialsFile(cfg.FirebaseCredentialsFile),
		)
		if err != nil {
			slog.Error("firebase NewApp failed", "error", err)
			os.Exit(1)
		}
		fbClient, err := fbApp.Auth(ctx)
		if err != nil {
			slog.Error("firebase Auth client failed", "error", fmt.Errorf("firebase Auth client: %w", err))
			os.Exit(1)
		}
		verifier = firebaseauth.NewVerifier(fbClient)
	}

	pool := database.Connect(cfg)
	defer pool.Close()

	instrumentStore := storage.NewInstrumentStore(pool)
	candleStore := storage.NewCandleStore(pool)
	userRepo := storage.NewUserStore(pool)
	tokenRepo := storage.NewRefreshTokenStore(pool)

	authSvc := services.NewAuthService(
		userRepo, tokenRepo, verifier,
		cfg.JWTSecret, cfg.AllowedFirebaseUIDs, cfg.Env,
	)
	authHandler := handlers.NewAuthHandler(authSvc, cfg.Env)

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

	routes.Register(r, pool, cfg, registry, authSvc, authHandler)

	slog.Info("server starting", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
