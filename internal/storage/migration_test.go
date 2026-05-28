package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
)

const migration016Version = 16

// migrationsDir returns the absolute path to the project's migrations/ directory by
// walking up from this test file's directory.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile: .../internal/storage/migration_test.go → go up to repo root
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("resolve migrations path: %v", err)
	}
	return filepath.Join(abs, "migrations")
}

// TestMigration016_DownGuard_RaisesWhenUserIDRowExists asserts that rolling back
// migration 016 fails with a RAISE EXCEPTION when a backtest_runs row has a non-null
// user_id column value (row 15).
//
// The test applies all migrations up to 016, inserts a row with user_id set, then
// attempts to step the migration down by one. The guarded down script must reject the
// rollback. The test is skipped when TEST_DATABASE_URL is unset.
func TestMigration016_DownGuard_RaisesWhenUserIDRowExists(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping DB-backed migration test")
	}

	migPath := "file://" + migrationsDir(t)

	m, err := migrate.New(migPath, dsn)
	if err != nil {
		t.Fatalf("create migrate instance: %v", err)
	}
	defer m.Close()

	// Apply all migrations up (idempotent in CI where migrations have already run).
	if upErr := m.Up(); upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", upErr)
	}

	// Obtain a pool to insert a backtest_runs row with user_id set.
	pool := newTestPool(t)

	// Insert a users row first (FK constraint).
	userID := uuid.New()
	_, err = pool.Exec(context.Background(),
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		userID, "mig016test+"+userID.String()+"@example.com", "mig test", "hash",
	)
	if err != nil {
		t.Fatalf("insert user for migration test: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID); cleanupErr != nil {
			t.Errorf("delete migration test user %s: %v", userID, cleanupErr)
		}
	})

	// Insert a backtest_run owned by that user so user_id IS NOT NULL.
	runID := uuid.New()
	_, err = pool.Exec(context.Background(),
		`INSERT INTO backtest_runs
		  (id, user_id, instrument_token, from_ts, to_ts, candle_interval, status, strategy_slug, created_at)
		 VALUES ($1, $2, $3, '2025-01-01', '2025-03-31', '1d', 'COMPLETED', $4, NOW())`,
		runID, userID, "NIFTY-FUT", "ma_crossover",
	)
	if err != nil {
		t.Fatalf("insert backtest_run for migration test: %v", err)
	}
	t.Cleanup(func() {
		if _, cleanupErr := pool.Exec(context.Background(), `DELETE FROM backtest_runs WHERE id = $1`, runID); cleanupErr != nil {
			t.Errorf("delete migration test backtest run %s: %v", runID, cleanupErr)
		}
	})

	// Re-open a fresh migrate instance — the first one may have cached the dirty state.
	m2, err := migrate.New(migPath, dsn)
	if err != nil {
		t.Fatalf("create second migrate instance: %v", err)
	}
	t.Cleanup(func() {
		m2.Close()
	})
	t.Cleanup(func() {
		version, dirty, versionErr := m2.Version()
		if versionErr != nil {
			t.Errorf("read migration state during cleanup: %v", versionErr)
			return
		}
		if dirty {
			if forceErr := m2.Force(migration016Version); forceErr != nil {
				t.Errorf("restore dirty migration state to version %d: %v", migration016Version, forceErr)
			}
			return
		}
		if version != migration016Version {
			if migrateErr := m2.Migrate(migration016Version); migrateErr != nil && !errors.Is(migrateErr, migrate.ErrNoChange) {
				t.Errorf("restore migration version %d: %v", migration016Version, migrateErr)
			}
		}
	})

	// Attempt to roll back one step. The guarded down SQL must RAISE EXCEPTION.
	downErr := m2.Steps(-1)
	if downErr == nil {
		t.Fatal("expected migrate down to fail due to 016 down guard, but it succeeded")
	}
	if !strings.Contains(downErr.Error(), "cannot drop backtest_runs.user_id") {
		t.Fatalf("migrate down failed for an unexpected reason: %v", downErr)
	}
	t.Logf("migrate down rejected as expected: %v", downErr)
}
