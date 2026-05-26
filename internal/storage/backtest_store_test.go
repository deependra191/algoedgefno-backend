package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// insertTestUser inserts a minimal users row and returns its ID.
// Tests call this once per user identity they need to isolate.
func insertTestUser(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (interface{ RowsAffected() int64 }, error)
}) uuid.UUID {
	t.Helper()
	return uuid.Nil // placeholder — real impl below
}

// insertTestUserReal inserts a users row and returns its ID.
func insertTestUserReal(t *testing.T, ctx context.Context, s *BacktestStore) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		id, "testuser+"+id.String()+"@example.com", "Test User", "hash",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		// DELETE cascade handles backtest_runs via FK.
		_, _ = s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

// insertTestBacktestRun inserts a minimal completed backtest_run owned by userID and returns it.
func insertTestBacktestRun(t *testing.T, ctx context.Context, s *BacktestStore, userID uuid.UUID) *models.BacktestRun {
	t.Helper()
	slug := "ma_crossover"
	capital := 200000.0
	pnl := 10000.0
	total := 5
	wins := 3
	run := &models.BacktestRun{
		ID:             uuid.New(),
		UserID:         &userID,
		InstrumentToken: "NIFTY-FUT",
		FromTs:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ToTs:           time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
		CandleInterval: models.CandleInterval1D,
		Status:         models.BacktestCompleted,
		StrategySlug:   &slug,
		Capital:        &capital,
		NetPnl:         &pnl,
		TotalTrades:    &total,
		WinCount:       &wins,
	}
	if err := s.Create(ctx, run); err != nil {
		t.Fatalf("create test backtest run: %v", err)
	}
	// Stamp completed_at and total_trades so it appears in ListCompleted.
	if _, err := s.pool.Exec(ctx,
		`UPDATE backtest_runs SET completed_at = NOW() WHERE id = $1`, run.ID,
	); err != nil {
		t.Fatalf("stamp completed_at: %v", err)
	}
	return run
}

// TestBacktestStore_GetByID_TenantIsolation asserts that GetByID returns ErrNotFound
// when the requesting userID differs from the run owner (row 10).
func TestBacktestStore_GetByID_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	s := NewBacktestStore(pool)
	ctx := context.Background()

	ownerID := insertTestUserReal(t, ctx, s)
	otherID := insertTestUserReal(t, ctx, s)

	run := insertTestBacktestRun(t, ctx, s, ownerID)

	// Owner can read their own run.
	got, err := s.GetByID(ctx, run.ID, ownerID)
	if err != nil {
		t.Fatalf("owner GetByID: unexpected error: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("expected ID %s, got %s", run.ID, got.ID)
	}

	// Other tenant cannot see owner's run.
	_, err = s.GetByID(ctx, run.ID, otherID)
	if err != models.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-tenant GetByID, got %v", err)
	}
}

// TestBacktestStore_GetByIDWithTrades_TenantIsolation asserts that GetByIDWithTrades
// returns ErrNotFound for a different tenant (row 11).
func TestBacktestStore_GetByIDWithTrades_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	s := NewBacktestStore(pool)
	ctx := context.Background()

	ownerID := insertTestUserReal(t, ctx, s)
	otherID := insertTestUserReal(t, ctx, s)

	run := insertTestBacktestRun(t, ctx, s, ownerID)

	// Owner can read their own run with trades.
	got, err := s.GetByIDWithTrades(ctx, run.ID, ownerID)
	if err != nil {
		t.Fatalf("owner GetByIDWithTrades: unexpected error: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("expected ID %s, got %s", run.ID, got.ID)
	}

	// Other tenant gets ErrNotFound.
	_, err = s.GetByIDWithTrades(ctx, run.ID, otherID)
	if err != models.ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-tenant GetByIDWithTrades, got %v", err)
	}
}

// TestBacktestStore_ListCompleted_TenantIsolation asserts that ListCompleted and its
// COUNT query exclude rows owned by other tenants (row 12).
func TestBacktestStore_ListCompleted_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	s := NewBacktestStore(pool)
	ctx := context.Background()

	ownerID := insertTestUserReal(t, ctx, s)
	otherID := insertTestUserReal(t, ctx, s)

	// Insert one run for each tenant.
	insertTestBacktestRun(t, ctx, s, ownerID)
	insertTestBacktestRun(t, ctx, s, otherID)

	// Owner's list must contain only their run; COUNT must match.
	runs, total, err := s.ListCompleted(ctx, ownerID, 1, 100)
	if err != nil {
		t.Fatalf("ListCompleted: %v", err)
	}
	for _, r := range runs {
		if r.UserID == nil || *r.UserID != ownerID {
			t.Errorf("ListCompleted returned run owned by a different user: %s", r.ID)
		}
	}
	if total != len(runs) {
		t.Errorf("total count %d does not match returned rows %d — COUNT and SELECT predicates are out of sync", total, len(runs))
	}

	// Other tenant's list must not include owner's rows.
	otherRuns, _, err := s.ListCompleted(ctx, otherID, 1, 100)
	if err != nil {
		t.Fatalf("ListCompleted (other): %v", err)
	}
	for _, r := range otherRuns {
		if r.UserID != nil && *r.UserID == ownerID {
			t.Errorf("other tenant's ListCompleted leaked owner's run %s", r.ID)
		}
	}
}

// TestBacktestStore_Create_PersistsUserID asserts that Create stores the supplied
// UserID in the user_id column — the ownership record that all tenant filters rely on (row 13).
func TestBacktestStore_Create_PersistsUserID(t *testing.T) {
	pool := newTestPool(t)
	s := NewBacktestStore(pool)
	ctx := context.Background()

	ownerID := insertTestUserReal(t, ctx, s)

	run := insertTestBacktestRun(t, ctx, s, ownerID)

	// Read back via SQL to verify user_id column value directly.
	var storedUserID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM backtest_runs WHERE id = $1`, run.ID,
	).Scan(&storedUserID)
	if err != nil {
		t.Fatalf("read user_id from DB: %v", err)
	}
	if storedUserID != ownerID {
		t.Errorf("expected user_id %s in DB, got %s", ownerID, storedUserID)
	}
}
