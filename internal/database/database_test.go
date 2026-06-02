package database

import (
	"errors"
	"strings"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

func TestDatabaseDSN_UsesRequireSSLModeWhenEnabled(t *testing.T) {
	cfg := &config.Config{
		DBHost:        "postgres",
		DBPort:        "5432",
		DBUser:        "algoedgefno_prod_app",
		DBPass:        "db_password",
		DBName:        "algoedgefno_prod",
		DBSSLRequired: true,
	}

	got := databaseDSN(cfg, sslModeRequire)
	want := "postgres://algoedgefno_prod_app:db_password@postgres:5432/algoedgefno_prod?sslmode=require"
	if got != want {
		t.Fatalf("databaseDSN() = %q, want %q", got, want)
	}
}

func TestDatabaseDSN_UsesDisableSSLModeWhenDisabled(t *testing.T) {
	cfg := &config.Config{
		DBHost:        "postgres",
		DBPort:        "5432",
		DBUser:        "algoedgefno_prod_app",
		DBPass:        "db_password",
		DBName:        "algoedgefno_prod",
		DBSSLRequired: false,
	}

	got := databaseDSN(cfg, sslModeDisable)
	want := "postgres://algoedgefno_prod_app:db_password@postgres:5432/algoedgefno_prod?sslmode=disable"
	if got != want {
		t.Fatalf("databaseDSN() = %q, want %q", got, want)
	}
}

func TestDatabaseDSN_ReturnsDatabaseURLVerbatim(t *testing.T) {
	cfg := &config.Config{
		DatabaseURL:   "postgres://user:pass@managed-db:5432/algoedgefno_prod?sslmode=verify-full",
		DBHost:        "postgres",
		DBPort:        "5432",
		DBUser:        "algoedgefno_prod_app",
		DBPass:        "db_password",
		DBName:        "algoedgefno_prod",
		DBSSLRequired: false,
	}

	got := databaseDSN(cfg, sslModeDisable)
	if got != cfg.DatabaseURL {
		t.Fatalf("databaseDSN() = %q, want verbatim DATABASE_URL %q", got, cfg.DatabaseURL)
	}
}

func TestSanitizeDatabaseErrorRedactsDSNAndSecrets(t *testing.T) {
	dsn := "postgres://app_user:db_password@postgres-prod:5432/algoedgefno_prod?sslmode=require"
	cfg := &config.Config{
		DatabaseURL: dsn,
		DBPass:      "db_password",
		JWTSecret:   "jwt_secret",
	}
	err := errors.New("failed with postgres://app_user:db_password@postgres-prod:5432/algoedgefno_prod?sslmode=require jwt_secret")

	got := sanitizeDatabaseError(err, cfg, dsn)
	for _, forbidden := range []string{dsn, cfg.DBPass, cfg.JWTSecret} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitizeDatabaseError() leaked %q in %q", forbidden, got)
		}
	}
}
