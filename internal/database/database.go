package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

const (
	sslModeRequire = "require"
	sslModeDisable = "disable"
)

// Connect creates a PostgreSQL pool and runs startup migrations when enabled by config.
//
// Postgres TLS is gated by cfg.DBSSLRequired (env DB_SSL_REQUIRED, default true).
// True selects sslmode=require, false selects sslmode=disable. When DATABASE_URL is
// set, its embedded sslmode wins and cfg.DBSSLRequired is ignored for that DSN.
func Connect(cfg *config.Config) *pgxpool.Pool {
	sslMode := sslModeRequire
	if !cfg.DBSSLRequired {
		sslMode = sslModeDisable
	}

	dsn := databaseDSN(cfg, sslMode)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("failed to create database connection pool: %s", sanitizeDatabaseError(err, cfg, dsn))
	}

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("failed to ping database (env=%s db_host=%s db_name=%s): %s",
			cfg.Env, cfg.DBHost, cfg.DBName, sanitizeDatabaseError(err, cfg, dsn))
	}

	if cfg.ShouldRunMigrations() {
		runMigrations(cfg.MigrationsPath, dsn, cfg)
		log.Println("database connected and migrations applied")
		return pool
	}

	log.Printf("database connected; automatic migrations disabled (env=%s db_host=%s db_name=%s)",
		cfg.Env, cfg.DBHost, cfg.DBName)
	return pool
}

func databaseDSN(cfg *config.Config, sslMode string) string {
	if cfg.DatabaseURL != "" {
		return cfg.DatabaseURL
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName, sslMode,
	)
}

func runMigrations(migrationsPath, dsn string, cfg *config.Config) {
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		log.Fatalf("failed to create migrate instance: %s", sanitizeDatabaseError(err, cfg, dsn))
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %s", sanitizeDatabaseError(err, cfg, dsn))
	}
}

func sanitizeDatabaseError(err error, cfg *config.Config, dsn string) string {
	msg := err.Error()
	if dsn != "" {
		msg = strings.ReplaceAll(msg, dsn, "[redacted]")
	}
	if cfg == nil {
		return msg
	}
	for _, secret := range []string{cfg.DBPass, cfg.DatabaseURL, cfg.JWTSecret} {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[redacted]")
		}
	}
	return msg
}
