package services

import (
	"context"
	"fmt"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// HealthService checks database connectivity and environment identity.
type HealthService struct {
	repo models.HealthRepository
	env  config.Environment
}

// NewHealthService creates a HealthService for the given environment.
func NewHealthService(repo models.HealthRepository, env config.Environment) *HealthService {
	return &HealthService{repo: repo, env: env}
}

// CheckReadiness verifies DB connectivity. For production and staging environments
// it also queries the environment_identity table and fails if the stored identity
// does not match the configured APP_ENV.
func (s *HealthService) CheckReadiness(ctx context.Context) error {
	if err := s.repo.Ping(ctx); err != nil {
		return fmt.Errorf("database unavailable")
	}

	if s.env == config.EnvProduction || s.env == config.EnvStaging {
		identity, err := s.repo.QueryDBIdentity(ctx)
		if err != nil {
			return fmt.Errorf("could not read database identity")
		}
		if config.Environment(identity) != s.env {
			return fmt.Errorf("environment identity check failed")
		}
	}

	return nil
}

// MigrationVersion returns the highest successfully applied migration version.
func (s *HealthService) MigrationVersion(ctx context.Context) (int64, error) {
	return s.repo.MigrationVersion(ctx)
}
