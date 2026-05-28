package routes

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mustOpenPool opens a pgxpool connection to dsn and registers a cleanup to close it.
func mustOpenPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}
