package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// insertTestUser inserts a users row and returns its ID.
func insertTestUser(t *testing.T, ctx context.Context, s *BacktestStore) uuid.UUID {
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
		if _, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
			t.Errorf("delete test user %s: %v", id, err)
		}
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
		ID:              uuid.New(),
		UserID:          &userID,
		InstrumentToken: "NIFTY-FUT",
		FromTs:          time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ToTs:            time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC),
		CandleInterval:  models.CandleInterval1D,
		Status:          models.BacktestCompleted,
		StrategySlug:    &slug,
		Capital:         &capital,
		NetPnl:          &pnl,
		TotalTrades:     &total,
		WinCount:        &wins,
	}
	if err := s.Create(ctx, run); err != nil {
		t.Fatalf("create test backtest run: %v", err)
	}
	t.Cleanup(func() {
		if _, err := s.pool.Exec(ctx, `DELETE FROM backtest_runs WHERE id = $1`, run.ID); err != nil {
			t.Errorf("delete test backtest run %s: %v", run.ID, err)
		}
	})
	if err := s.UpdateResult(ctx, run); err != nil {
		t.Fatalf("complete test backtest run: %v", err)
	}
	return run
}

// TestBacktestStore_GetByID_TenantIsolation asserts that GetByID returns ErrNotFound
// when the requesting userID differs from the run owner (row 10).
func TestBacktestStore_GetByID_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	s := NewBacktestStore(pool)
	ctx := context.Background()

	ownerID := insertTestUser(t, ctx, s)
	otherID := insertTestUser(t, ctx, s)

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

	ownerID := insertTestUser(t, ctx, s)
	otherID := insertTestUser(t, ctx, s)

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

	ownerID := insertTestUser(t, ctx, s)
	otherID := insertTestUser(t, ctx, s)

	// Insert one run for each tenant.
	ownerRun := insertTestBacktestRun(t, ctx, s, ownerID)
	otherRun := insertTestBacktestRun(t, ctx, s, otherID)

	// Owner's list must contain only their run; COUNT must match.
	runs, total, err := s.ListCompleted(ctx, ownerID, 1, 100)
	if err != nil {
		t.Fatalf("ListCompleted: %v", err)
	}
	if total != 1 || len(runs) != 1 {
		t.Fatalf("owner ListCompleted returned total=%d rows=%d, want exactly one owned row", total, len(runs))
	}
	if runs[0].ID != ownerRun.ID || runs[0].UserID == nil || *runs[0].UserID != ownerID {
		t.Errorf("owner ListCompleted returned run %s for user %v, want run %s for user %s", runs[0].ID, runs[0].UserID, ownerRun.ID, ownerID)
	}

	// Other tenant's list must not include owner's rows.
	otherRuns, otherTotal, err := s.ListCompleted(ctx, otherID, 1, 100)
	if err != nil {
		t.Fatalf("ListCompleted (other): %v", err)
	}
	if otherTotal != 1 || len(otherRuns) != 1 {
		t.Fatalf("other ListCompleted returned total=%d rows=%d, want exactly one owned row", otherTotal, len(otherRuns))
	}
	if otherRuns[0].ID != otherRun.ID || otherRuns[0].UserID == nil || *otherRuns[0].UserID != otherID {
		t.Errorf("other ListCompleted returned run %s for user %v, want run %s for user %s", otherRuns[0].ID, otherRuns[0].UserID, otherRun.ID, otherID)
	}
}

// TestBacktestStore_Create_PersistsUserID asserts that Create stores the supplied
// UserID in the user_id column — the ownership record that all tenant filters rely on (row 13).
func TestBacktestStore_Create_PersistsUserID(t *testing.T) {
	pool := newTestPool(t)
	s := NewBacktestStore(pool)
	ctx := context.Background()

	ownerID := insertTestUser(t, ctx, s)

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
