package main

import (
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

// TestValidateServerConfig_EmptyAllowlistProdFails asserts that a production
// config with an empty ALLOWED_FIREBASE_UIDS list is rejected by
// ValidateServerConfig. This tests the fail-closed startup behavior that
// prevents the server from launching without an allowlist in production.
func TestValidateServerConfig_EmptyAllowlistProdFails(t *testing.T) {
	cfg := &config.Config{
		Env:                 config.EnvProduction,
		FirebaseWebAPIKey:   "web-api-key",
		AllowedFirebaseUIDs: nil, // empty — must reject
	}
	if err := config.ValidateServerConfig(cfg); err == nil {
		t.Fatal("expected ValidateServerConfig to fail with empty allowlist in production")
	}
}

// TestValidateServerConfig_UnreadableCredentialsFileFails asserts that startup
// fails if FIREBASE_CREDENTIALS_FILE points to a non-existent path.
func TestValidateServerConfig_UnreadableCredentialsFileFails(t *testing.T) {
	cfg := &config.Config{
		Env:                     config.EnvDevelopment,
		FirebaseProjectID:       "my-project",
		FirebaseCredentialsFile: "/nonexistent/path/credentials.json",
	}
	if err := config.ValidateServerConfig(cfg); err == nil {
		t.Fatal("expected ValidateServerConfig to fail for unreadable credentials file")
	}
}

// TestValidateServerConfig_MissingProjectIDWithCredsFails asserts that
// providing a credentials file without a project ID is rejected.
func TestValidateServerConfig_MissingProjectIDWithCredsFails(t *testing.T) {
	cfg := &config.Config{
		Env:                     config.EnvDevelopment,
		FirebaseProjectID:       "",
		FirebaseCredentialsFile: "/some/file.json", // project ID check fires before readable check
	}
	if err := config.ValidateServerConfig(cfg); err == nil {
		t.Fatal("expected error when project ID is missing but credentials file is set")
	}
}
