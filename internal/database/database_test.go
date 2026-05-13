package database

import (
	"errors"
	"strings"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

func TestSanitizeDatabaseErrorRedactsDSNAndSecrets(t *testing.T) {
	dsn := "postgres://app_user:db_password@postgres-prod:5432/algoedgefno_prod?sslmode=require"
	cfg := &config.Config{
		DatabaseURL:    dsn,
		DBPass:         "db_password",
		AppSecretToken: "app_token",
		JWTSecret:      "jwt_secret",
	}
	err := errors.New("failed with postgres://app_user:db_password@postgres-prod:5432/algoedgefno_prod?sslmode=require app_token jwt_secret")

	got := sanitizeDatabaseError(err, cfg, dsn)
	for _, forbidden := range []string{dsn, cfg.DBPass, cfg.AppSecretToken, cfg.JWTSecret} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitizeDatabaseError() leaked %q in %q", forbidden, got)
		}
	}
}
