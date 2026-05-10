package models

import "context"

// HealthRepository provides database-level health and identity checks.
// Storage returns raw values; the service layer enforces identity invariants.
type HealthRepository interface {
	// Ping verifies the database connection is alive.
	Ping(ctx context.Context) error

	// QueryDBIdentity returns the value stored in the environment_identity table.
	// Returns an error if the table does not exist or contains no row.
	QueryDBIdentity(ctx context.Context) (string, error)

	// MigrationVersion returns the highest successfully applied migration version.
	// Returns 0 and no error when schema_migrations exists but has no applied rows.
	MigrationVersion(ctx context.Context) (int64, error)
}
