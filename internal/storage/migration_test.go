package storage

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration version constants used across tests.
const (
	migration016Version = 16
	migration017Version = 17
	migration018Version = 18
)

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

// newMigrate opens a fresh migrate.Migrate instance pointed at the given DSN and the
// project migrations directory. Callers are responsible for calling Close.
func newMigrate(t *testing.T, dsn string) *migrate.Migrate {
	t.Helper()
	migPath := "file://" + migrationsDir(t)
	m, err := migrate.New(migPath, dsn)
	if err != nil {
		t.Fatalf("create migrate instance: %v", err)
	}
	return m
}

// newIsolatedPool opens a pgxpool.Pool against the given DSN (typically an isolated DB
// from newIsolatedMigrationDB). It registers cleanup to close the pool.
func newIsolatedPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect isolated test db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestMigration016_DownGuard_RaisesWhenUserIDRowExists asserts that rolling back
// migration 016 fails with a RAISE EXCEPTION when a backtest_runs row has a non-null
// user_id column value.
//
// The test applies migrations up to 016, inserts a backtest_runs row with user_id set,
// then attempts to step the migration down by one. The guarded down script must reject
// the rollback. Uses an isolated database so the shared CI database is never left in a
// partial-migration state.
func TestMigration016_DownGuard_RaisesWhenUserIDRowExists(t *testing.T) {
	isolatedDSN, cleanup := newIsolatedMigrationDB(t)
	defer cleanup()

	m := newMigrate(t, isolatedDSN)
	defer m.Close()

	// Apply migrations up to exactly version 016 (version-targeted).
	if err := m.Migrate(migration016Version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate to v%d: %v", migration016Version, err)
	}

	pool := newIsolatedPool(t, isolatedDSN)

	// Insert a users row (pre-017 fixture: name and password_hash NOT NULL, no firebase_uid).
	userID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		userID, "mig016test+"+userID.String()+"@example.com", "mig test", "hash",
	)
	if err != nil {
		t.Fatalf("insert user for migration test: %v", err)
	}

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

	// Open a second migrate instance to perform the rollback attempt — the first one may
	// have cached schema state.
	m2 := newMigrate(t, isolatedDSN)
	defer m2.Close()

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

// TestMigration017_UpGuard_RaisesWhenUsersRowExists asserts that applying migration 017
// fails with a RAISE EXCEPTION when the users table already contains rows.
//
// The guard is intentional: there is no legitimate path that creates a users row before
// 017 applies. Any pre-existing row is an operational anomaly; auto-linking is prohibited.
// The test also verifies that schema_migrations is left dirty at version 17 after the
// failed apply, confirming golang-migrate's dirty-flag behaviour.
func TestMigration017_UpGuard_RaisesWhenUsersRowExists(t *testing.T) {
	isolatedDSN, cleanup := newIsolatedMigrationDB(t)
	defer cleanup()

	m := newMigrate(t, isolatedDSN)
	defer m.Close()

	// Apply up to v016 (pre-017 schema).
	if err := m.Migrate(migration016Version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate to v%d: %v", migration016Version, err)
	}

	pool := newIsolatedPool(t, isolatedDSN)

	// Seed a pre-017 users row (name and password_hash are NOT NULL at v16; firebase_uid
	// does not exist yet and must not be referenced).
	userID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		userID, "mig017guard+"+userID.String()+"@example.com", "stub name", "stub hash",
	)
	if err != nil {
		t.Fatalf("insert stub user at v16: %v", err)
	}

	// Attempt to apply 017. The inline DO-block guard must RAISE EXCEPTION.
	m2 := newMigrate(t, isolatedDSN)
	defer m2.Close()

	upErr := m2.Steps(1)
	if upErr == nil {
		t.Fatal("expected migrate 017 up to fail due to zero-rows guard, but it succeeded")
	}
	if !strings.Contains(upErr.Error(), "migration 017: users table must be empty") {
		t.Fatalf("migrate 017 up failed for an unexpected reason: %v", upErr)
	}
	t.Logf("migrate 017 up rejected as expected: %v", upErr)

	// Verify golang-migrate left the dirty flag set at version 17.
	m3 := newMigrate(t, isolatedDSN)
	defer m3.Close()

	version, dirty, versionErr := m3.Version()
	if versionErr != nil {
		t.Fatalf("read migration version after guard failure: %v", versionErr)
	}
	if version != migration017Version {
		t.Errorf("expected schema_migrations version %d after dirty guard, got %d", migration017Version, version)
	}
	if !dirty {
		t.Errorf("expected schema_migrations dirty=true after 017 guard raised, got dirty=false")
	}
	t.Logf("confirmed schema_migrations version=%d dirty=%v", version, dirty)
}

// TestMigration017_DirtyStateRecovery exercises the full operator recovery cycle for a
// migration 017 dirty state:
//  1. Apply to v016.
//  2. Seed a stub users row.
//  3. Attempt 017 — guard raises; schema_migrations becomes dirty at v17.
//  4. Force schema_migrations back to v016 (simulating `migrate force 16`).
//  5. Delete the stub row.
//  6. Re-apply 017 — must succeed now that users is empty.
func TestMigration017_DirtyStateRecovery(t *testing.T) {
	isolatedDSN, cleanup := newIsolatedMigrationDB(t)
	defer cleanup()

	// Step 1: apply to v016.
	m := newMigrate(t, isolatedDSN)
	defer m.Close()

	if err := m.Migrate(migration016Version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("step 1 — migrate to v%d: %v", migration016Version, err)
	}

	pool := newIsolatedPool(t, isolatedDSN)

	// Step 2: seed a stub users row at v16.
	userID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		userID, "mig017recovery+"+userID.String()+"@example.com", "recovery name", "recovery hash",
	)
	if err != nil {
		t.Fatalf("step 2 — insert stub user: %v", err)
	}

	// Step 3: attempt 017 — must fail.
	mUp := newMigrate(t, isolatedDSN)
	defer mUp.Close()

	if upErr := mUp.Steps(1); upErr == nil {
		t.Fatal("step 3 — expected 017 up to fail due to guard, but succeeded")
	}

	// Verify dirty at v17.
	mCheck := newMigrate(t, isolatedDSN)
	defer mCheck.Close()

	ver, dirty, _ := mCheck.Version()
	if ver != migration017Version || !dirty {
		t.Fatalf("step 3 — expected version=%d dirty=true, got version=%d dirty=%v", migration017Version, ver, dirty)
	}

	// Step 4: force schema_migrations to v016 (operator `migrate force 16`).
	mForce := newMigrate(t, isolatedDSN)
	defer mForce.Close()

	if forceErr := mForce.Force(migration016Version); forceErr != nil {
		t.Fatalf("step 4 — force to v%d: %v", migration016Version, forceErr)
	}

	ver2, dirty2, _ := mForce.Version()
	if ver2 != migration016Version || dirty2 {
		t.Fatalf("step 4 — expected version=%d dirty=false after force, got version=%d dirty=%v", migration016Version, ver2, dirty2)
	}

	// Step 5: delete the stub row so the guard will pass.
	if _, delErr := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID); delErr != nil {
		t.Fatalf("step 5 — delete stub user: %v", delErr)
	}

	// Step 6: re-apply 017 — must succeed.
	mReapply := newMigrate(t, isolatedDSN)
	defer mReapply.Close()

	if reapplyErr := mReapply.Steps(1); reapplyErr != nil {
		t.Fatalf("step 6 — re-apply 017 after dirty-state recovery: %v", reapplyErr)
	}

	ver3, dirty3, _ := mReapply.Version()
	if ver3 != migration017Version || dirty3 {
		t.Fatalf("step 6 — expected version=%d dirty=false, got version=%d dirty=%v", migration017Version, ver3, dirty3)
	}
	t.Logf("dirty-state recovery cycle completed successfully at version %d", ver3)
}

// TestMigration017_DownGuard_RaisesWhenFirebaseUIDRowExists asserts that rolling back
// migration 017 fails with a RAISE EXCEPTION when any users row has firebase_uid set.
func TestMigration017_DownGuard_RaisesWhenFirebaseUIDRowExists(t *testing.T) {
	isolatedDSN, cleanup := newIsolatedMigrationDB(t)
	defer cleanup()

	m := newMigrate(t, isolatedDSN)
	defer m.Close()

	// Apply up to v017 (post-017 schema: firebase_uid exists; name/password_hash nullable).
	if err := m.Migrate(migration017Version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate to v%d: %v", migration017Version, err)
	}

	pool := newIsolatedPool(t, isolatedDSN)

	// Insert a post-017 users row (firebase_uid required; name and password_hash may be NULL).
	userID := uuid.New()
	firebaseUID := "test-uid-" + uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, firebase_uid, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())`,
		userID, "mig017down+"+userID.String()+"@example.com", firebaseUID,
	)
	if err != nil {
		t.Fatalf("insert post-017 user: %v", err)
	}

	// Attempt to roll back 017. The guarded down SQL must RAISE EXCEPTION.
	m2 := newMigrate(t, isolatedDSN)
	defer m2.Close()

	downErr := m2.Steps(-1)
	if downErr == nil {
		t.Fatal("expected migrate 017 down to fail due to down guard, but it succeeded")
	}
	if !strings.Contains(downErr.Error(), "cannot roll back migration 017") {
		t.Fatalf("migrate 017 down failed for an unexpected reason: %v", downErr)
	}
	t.Logf("migrate 017 down rejected as expected: %v", downErr)
}

// TestMigration018_DownGuard_RaisesWhenRefreshTokensRowExists asserts that rolling back
// migration 018 fails with a RAISE EXCEPTION when any refresh_tokens row exists,
// including rows where revoked_at is set.
func TestMigration018_DownGuard_RaisesWhenRefreshTokensRowExists(t *testing.T) {
	isolatedDSN, cleanup := newIsolatedMigrationDB(t)
	defer cleanup()

	m := newMigrate(t, isolatedDSN)
	defer m.Close()

	// Apply up to v018.
	if err := m.Migrate(migration018Version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate to v%d: %v", migration018Version, err)
	}

	pool := newIsolatedPool(t, isolatedDSN)

	// Insert a post-017 users row (firebase_uid required).
	userID := uuid.New()
	firebaseUID := "test-uid-" + uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, firebase_uid, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())`,
		userID, "mig018down+"+userID.String()+"@example.com", firebaseUID,
	)
	if err != nil {
		t.Fatalf("insert user for 018 guard test: %v", err)
	}

	// Insert an active refresh_tokens row.
	activeTokenID := uuid.New()
	_, err = pool.Exec(context.Background(),
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, NOW() + interval '7 days', NOW())`,
		activeTokenID, userID, "hash-active-"+uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("insert active refresh_token: %v", err)
	}

	// Insert a revoked refresh_tokens row (revoked_at set) — the guard must block even
	// when only revoked rows exist.
	revokedTokenID := uuid.New()
	_, err = pool.Exec(context.Background(),
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked_at, created_at)
		 VALUES ($1, $2, $3, NOW() + interval '7 days', NOW(), NOW())`,
		revokedTokenID, userID, "hash-revoked-"+uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("insert revoked refresh_token: %v", err)
	}

	// Attempt to roll back 018. The guarded down SQL must RAISE EXCEPTION.
	m2 := newMigrate(t, isolatedDSN)
	defer m2.Close()

	downErr := m2.Steps(-1)
	if downErr == nil {
		t.Fatal("expected migrate 018 down to fail due to down guard, but it succeeded")
	}
	if !strings.Contains(downErr.Error(), "cannot roll back migration 018") {
		t.Fatalf("migrate 018 down failed for an unexpected reason: %v", downErr)
	}
	t.Logf("migrate 018 down rejected as expected: %v", downErr)
}
