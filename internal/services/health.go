package services

import (
	"context"
	"fmt"
	"time"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// readinessTimeout caps how long a single readiness probe may wait for DB responses.
const readinessTimeout = 2 * time.Second

// ReadinessChecks holds the per-check status returned on a successful readiness probe.
// Database is always "ok". DatabaseIdentity is "ok" for production and staging,
// or "skipped" for development and test environments.
type ReadinessChecks struct {
	Database         string
	DatabaseIdentity string
}

// HealthService checks database connectivity and environment identity.
type HealthService struct {
	repo models.HealthRepository
	env  config.Environment
}

// NewHealthService creates a HealthService for the given environment.
func NewHealthService(repo models.HealthRepository, env config.Environment) *HealthService {
	return &HealthService{repo: repo, env: env}
}

// CheckReadiness verifies DB connectivity within a bounded timeout. For production and staging
// environments it also queries the environment_identity table and fails if the stored identity
// does not match the configured APP_ENV.
func (s *HealthService) CheckReadiness(ctx context.Context) (ReadinessChecks, error) {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()

	if err := s.repo.Ping(ctx); err != nil {
		return ReadinessChecks{}, fmt.Errorf("database unavailable")
	}

	if s.env == config.EnvProduction || s.env == config.EnvStaging {
		identity, err := s.repo.QueryDBIdentity(ctx)
		if err != nil {
			return ReadinessChecks{}, fmt.Errorf("could not read database identity")
		}
		if config.Environment(identity) != s.env {
			return ReadinessChecks{}, fmt.Errorf("environment identity check failed")
		}
		return ReadinessChecks{Database: "ok", DatabaseIdentity: "ok"}, nil
	}

	return ReadinessChecks{Database: "ok", DatabaseIdentity: "skipped"}, nil
}

// MigrationVersion returns the highest successfully applied migration version.
func (s *HealthService) MigrationVersion(ctx context.Context) (int64, error) {
	return s.repo.MigrationVersion(ctx)
}
