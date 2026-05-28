package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// recordingBacktestRepo wraps mockBacktestRepoForStrategy and additionally records
// the userID argument passed to LatestCompletedBySlug so tests can assert tenant
// scoping is wired end-to-end through the service layer.
type recordingBacktestRepo struct {
	mockBacktestRepoForStrategy
	capturedSlug   string
	capturedUserID uuid.UUID
}

func (r *recordingBacktestRepo) LatestCompletedBySlug(ctx context.Context, slug string, userID uuid.UUID) (*models.BacktestRun, error) {
	r.capturedSlug = slug
	r.capturedUserID = userID
	return r.mockBacktestRepoForStrategy.LatestCompletedBySlug(ctx, slug, userID)
}

// TestGetBySlug_PassesUserIDToRepo asserts that StrategyService.GetBySlug forwards
// the userID argument to backtestRepo.LatestCompletedBySlug unchanged (row 14).
// This is the mock-level proof that the service does not hard-code a user identity or
// drop the parameter on the way to storage.
func TestGetBySlug_PassesUserIDToRepo(t *testing.T) {
	userIDA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	repo := &recordingBacktestRepo{
		mockBacktestRepoForStrategy: mockBacktestRepoForStrategy{
			latestBySlug: map[string]*models.BacktestRun{},
		},
	}

	svc := NewStrategyService(defaultLookup(), repo, &mockCandleRepoForStrategy{})

	_, err := svc.GetBySlug(context.Background(), testSlug, userIDA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.capturedUserID != userIDA {
		t.Errorf("expected userID %s passed to LatestCompletedBySlug, got %s", userIDA, repo.capturedUserID)
	}
	if repo.capturedSlug != testSlug {
		t.Errorf("expected slug %q passed to LatestCompletedBySlug, got %q", testSlug, repo.capturedSlug)
	}
}

// TestGetBySlug_TenantIsolation_LastBacktestNilForOtherTenant asserts that when the
// mock returns ErrNotFound for userIDB (only userIDA owns a completed run), the service
// exposes LastBacktest == nil in the result for userIDB (row 14).
func TestGetBySlug_TenantIsolation_LastBacktestNilForOtherTenant(t *testing.T) {
	userIDA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	userIDB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	runForA := &models.BacktestRun{ID: uuid.New(), Status: models.BacktestCompleted}

	// userIDA-scoped repo: returns runForA only when slug matches.
	repoA := &recordingBacktestRepo{
		mockBacktestRepoForStrategy: mockBacktestRepoForStrategy{
			latestBySlug: map[string]*models.BacktestRun{testSlug: runForA},
		},
	}
	// userIDB-scoped repo: no runs (simulates ErrNotFound for any userID != userIDA).
	repoB := &recordingBacktestRepo{
		mockBacktestRepoForStrategy: mockBacktestRepoForStrategy{
			latestBySlug: map[string]*models.BacktestRun{},
		},
	}

	// Verify userIDA gets lastBacktest populated.
	svcA := NewStrategyService(defaultLookup(), repoA, &mockCandleRepoForStrategy{})
	detailA, err := svcA.GetBySlug(context.Background(), testSlug, userIDA)
	if err != nil {
		t.Fatalf("userIDA GetBySlug: %v", err)
	}
	if detailA.LastBacktest == nil {
		t.Fatal("expected LastBacktest to be populated for userIDA who owns a completed run")
	}

	// Verify userIDB gets nil lastBacktest — they own no completed run.
	svcB := NewStrategyService(defaultLookup(), repoB, &mockCandleRepoForStrategy{})
	detailB, err := svcB.GetBySlug(context.Background(), testSlug, userIDB)
	if err != nil {
		t.Fatalf("userIDB GetBySlug: %v", err)
	}
	if detailB.LastBacktest != nil {
		t.Errorf("expected nil LastBacktest for userIDB who owns no run, got %+v", detailB.LastBacktest)
	}
	if repoB.capturedUserID != userIDB {
		t.Errorf("expected userID %s passed to repo for userIDB, got %s", userIDB, repoB.capturedUserID)
	}
}
