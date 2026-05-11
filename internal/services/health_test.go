package services

import (
	"context"
	"errors"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

// fakeHealthRepo implements models.HealthRepository for service tests.
type fakeHealthRepo struct {
	pingErr       error
	identity      string
	identityErr   error
	migVersion    int64
	migVersionErr error
}

func (f *fakeHealthRepo) Ping(_ context.Context) error { return f.pingErr }
func (f *fakeHealthRepo) QueryDBIdentity(_ context.Context) (string, error) {
	return f.identity, f.identityErr
}
func (f *fakeHealthRepo) MigrationVersion(_ context.Context) (int64, error) {
	return f.migVersion, f.migVersionErr
}

func TestCheckReadiness_PingFailure(t *testing.T) {
	svc := NewHealthService(&fakeHealthRepo{pingErr: errors.New("connection refused")}, config.EnvDevelopment)
	_, err := svc.CheckReadiness(context.Background())
	if err == nil {
		t.Fatal("expected error on ping failure, got nil")
	}
}

func TestCheckReadiness_DevEnv_SkipsIdentity(t *testing.T) {
	// development must not query identity even if the repo would return one.
	svc := NewHealthService(&fakeHealthRepo{identity: "staging"}, config.EnvDevelopment)
	checks, err := svc.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.Database != "ok" {
		t.Errorf("Database: got %q want %q", checks.Database, "ok")
	}
	if checks.DatabaseIdentity != "skipped" {
		t.Errorf("DatabaseIdentity: got %q want %q", checks.DatabaseIdentity, "skipped")
	}
}

func TestCheckReadiness_TestEnv_SkipsIdentity(t *testing.T) {
	svc := NewHealthService(&fakeHealthRepo{identity: "production"}, config.EnvTest)
	checks, err := svc.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.DatabaseIdentity != "skipped" {
		t.Errorf("DatabaseIdentity: got %q want %q", checks.DatabaseIdentity, "skipped")
	}
}

func TestCheckReadiness_StagingIdentityMatch(t *testing.T) {
	svc := NewHealthService(&fakeHealthRepo{identity: "staging"}, config.EnvStaging)
	checks, err := svc.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.Database != "ok" {
		t.Errorf("Database: got %q want %q", checks.Database, "ok")
	}
	if checks.DatabaseIdentity != "ok" {
		t.Errorf("DatabaseIdentity: got %q want %q", checks.DatabaseIdentity, "ok")
	}
}

func TestCheckReadiness_ProductionIdentityMatch(t *testing.T) {
	svc := NewHealthService(&fakeHealthRepo{identity: "production"}, config.EnvProduction)
	checks, err := svc.CheckReadiness(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.DatabaseIdentity != "ok" {
		t.Errorf("DatabaseIdentity: got %q want %q", checks.DatabaseIdentity, "ok")
	}
}

func TestCheckReadiness_StagingIdentityMismatch(t *testing.T) {
	// DB says "production" but app is configured for staging — must fail.
	svc := NewHealthService(&fakeHealthRepo{identity: "production"}, config.EnvStaging)
	_, err := svc.CheckReadiness(context.Background())
	if err == nil {
		t.Fatal("expected error on identity mismatch, got nil")
	}
}

func TestCheckReadiness_ProductionIdentityMismatch(t *testing.T) {
	// DB says "staging" but app is configured for production — must fail.
	svc := NewHealthService(&fakeHealthRepo{identity: "staging"}, config.EnvProduction)
	_, err := svc.CheckReadiness(context.Background())
	if err == nil {
		t.Fatal("expected error on identity mismatch, got nil")
	}
}

func TestCheckReadiness_IdentityQueryFails(t *testing.T) {
	// Missing or unprovisioned environment_identity table must fail readiness.
	svc := NewHealthService(
		&fakeHealthRepo{identityErr: errors.New("relation does not exist")},
		config.EnvProduction,
	)
	_, err := svc.CheckReadiness(context.Background())
	if err == nil {
		t.Fatal("expected error on identity query failure, got nil")
	}
}

func TestCheckReadiness_EmptyIdentityRow(t *testing.T) {
	// Table exists but has no row (provisioning was not done).
	svc := NewHealthService(
		&fakeHealthRepo{identityErr: errors.New("no rows in result set")},
		config.EnvStaging,
	)
	_, err := svc.CheckReadiness(context.Background())
	if err == nil {
		t.Fatal("expected error when identity row is missing, got nil")
	}
}

func TestMigrationVersion_Success(t *testing.T) {
	svc := NewHealthService(&fakeHealthRepo{migVersion: 11}, config.EnvDevelopment)
	v, err := svc.MigrationVersion(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 11 {
		t.Errorf("got %d want 11", v)
	}
}

func TestMigrationVersion_Failure(t *testing.T) {
	svc := NewHealthService(&fakeHealthRepo{migVersionErr: errors.New("query error")}, config.EnvDevelopment)
	_, err := svc.MigrationVersion(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
