package database

import (
	"context"
	"fmt"
	"log"

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

func Connect(cfg *config.Config) *pgxpool.Pool {
	sslMode := sslModeRequire
	if cfg.Env != config.EnvProduction {
		sslMode = sslModeDisable
	}

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName, sslMode,
	)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("failed to create connection pool: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	runMigrations(cfg.MigrationsPath, dsn)

	log.Println("database connected and migrations applied")
	return pool
}

func runMigrations(migrationsPath, dsn string) {
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %v", err)
	}
}
