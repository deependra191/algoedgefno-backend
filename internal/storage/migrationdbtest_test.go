package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// newIsolatedMigrationDB creates a fresh, uniquely-named PostgreSQL database derived
// from TEST_DATABASE_URL and returns a DSN pointing at it along with a cleanup function
// that drops the database on test completion.
//
// Destructive migration tests (apply/rollback cycles) must use this helper instead of
// the shared CI database, which is left at post-up state for non-destructive tests. The
// isolated database is created without TimescaleDB extension preloading — migration SQL
// must not assume it; the users/backtest schema used in guard tests does not require it.
//
// The test is skipped when TEST_DATABASE_URL is unset.
func newIsolatedMigrationDB(t *testing.T) (dsn string, cleanup func()) {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed migration test")
	}

	name := "mig_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	adminConn, err := pgx.Connect(context.Background(), base)
	if err != nil {
		t.Fatalf("newIsolatedMigrationDB: connect to base DSN: %v", err)
	}
	defer adminConn.Close(context.Background())

	// CREATE DATABASE cannot run inside a transaction; pgx uses implicit transactions
	// for single-query execution which is fine here.
	if _, err := adminConn.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %s", name)); err != nil {
		t.Fatalf("newIsolatedMigrationDB: create database %s: %v", name, err)
	}

	// Build the isolated DSN by replacing the database name component of the base URL.
	// The base URL is expected to have the form postgres://user[:pass]@host:port/dbname[?params].
	// We locate the last '/' before any '?' to find the dbname segment.
	isolatedDSN := replaceDBName(base, name)

	cleanup = func() {
		// Connect back to the admin (base) DB to drop the isolated one.
		conn, connErr := pgx.Connect(context.Background(), base)
		if connErr != nil {
			t.Errorf("newIsolatedMigrationDB cleanup: connect to base DSN: %v", connErr)
			return
		}
		defer conn.Close(context.Background())

		// Force-disconnect any lingering connections to the isolated DB before dropping.
		_, _ = conn.Exec(context.Background(),
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
			name,
		)
		if _, dropErr := conn.Exec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", name)); dropErr != nil {
			t.Errorf("newIsolatedMigrationDB cleanup: drop database %s: %v", name, dropErr)
		}
	}

	return isolatedDSN, cleanup
}

// replaceDBName swaps the database name segment of a postgres DSN with newName.
// It handles both postgres://... URIs and host=... keyword DSNs by delegating to
// pgx config parsing only for the keyword form; for URI form the path segment is
// rewritten directly.
func replaceDBName(dsn, newName string) string {
	// URI form: postgres://user@host:port/dbname?params
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		// Find the path segment: everything after the third '/' up to '?'.
		schemeEnd := strings.Index(dsn, "://") + 3
		rest := dsn[schemeEnd:]
		slashIdx := strings.Index(rest, "/")
		if slashIdx == -1 {
			// No path — just append /newName.
			return dsn + "/" + newName
		}
		authority := rest[:slashIdx+1] // everything up to and including the '/'
		afterSlash := rest[slashIdx+1:]
		// Strip existing dbname (up to '?' or end).
		if qIdx := strings.Index(afterSlash, "?"); qIdx != -1 {
			return dsn[:schemeEnd] + authority + newName + afterSlash[qIdx:]
		}
		return dsn[:schemeEnd] + authority + newName
	}
	// Keyword form: fallback — append dbname= override (pgx last-wins).
	return dsn + " dbname=" + newName
}
