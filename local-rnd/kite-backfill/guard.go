package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

func validateLocalConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if cfg.Env != config.EnvDevelopment && cfg.Env != config.EnvTest {
		return fmt.Errorf("kite backfill is local-only: APP_ENV must be development or test")
	}
	if hasUnsafeMarker(cfg.DatabaseURL, cfg.DBName, cfg.DBUser, cfg.DBHost) {
		return fmt.Errorf("kite backfill refused staging/prod-like database identity")
	}
	if !isLocalDatabaseHost(cfg.DBHost) {
		return fmt.Errorf("kite backfill refused non-local database host")
	}
	return nil
}

func ensureLocalDatabaseIdentity(ctx context.Context, pool *pgxpool.Pool) error {
	var identity string
	err := pool.QueryRow(ctx, `SELECT identity FROM environment_identity`).Scan(&identity)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if isUndefinedTable(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query database identity: %w", err)
	}
	if strings.TrimSpace(identity) == "" {
		return nil
	}
	return fmt.Errorf("kite backfill refused database identity %q", identity)
}

func hasUnsafeMarker(values ...string) bool {
	markers := []string{
		unsafeMarkerProduction,
		unsafeMarkerProd,
		unsafeMarkerStaging,
		unsafeMarkerStage,
		unsafeMarkerVPS,
	}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		for _, marker := range markers {
			if strings.Contains(normalized, marker) {
				return true
			}
		}
	}
	return false
}

func isLocalDatabaseHost(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	switch normalized {
	case localHostName, localIPv4Loopback, localIPv6Loopback:
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && ip.IsLoopback()
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgCodeNoSuchTable
}
