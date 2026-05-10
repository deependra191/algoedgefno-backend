package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthStore implements models.HealthRepository using a pgxpool.
type HealthStore struct {
	pool *pgxpool.Pool
}

// NewHealthStore creates a HealthStore backed by pool.
func NewHealthStore(pool *pgxpool.Pool) *HealthStore {
	return &HealthStore{pool: pool}
}

// Ping verifies the database connection is alive.
func (s *HealthStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// QueryDBIdentity returns the identity stored in the environment_identity table.
func (s *HealthStore) QueryDBIdentity(ctx context.Context) (string, error) {
	var identity string
	err := s.pool.QueryRow(ctx, `SELECT identity FROM environment_identity LIMIT 1`).Scan(&identity)
	return identity, err
}

// MigrationVersion returns the highest successfully applied migration version.
// COALESCE ensures a row is always returned; 0 is returned when no clean migration exists.
func (s *HealthStore) MigrationVersion(ctx context.Context) (int64, error) {
	var version int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = false`,
	).Scan(&version)
	return version, err
}
