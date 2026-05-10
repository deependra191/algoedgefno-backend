package handlers

import (
	"encoding/json"
	"testing"
)

// TestReadyResponseJSONShape asserts the wire shape of readyResponse is exactly {status, db}.
func TestReadyResponseJSONShape(t *testing.T) {
	resp := readyResponse{Status: "ok", DB: "connected"}
	keys := jsonKeys(t, resp)
	want := []string{"db", "status"}
	assertKeysEqual(t, keys, want)
}

// TestVersionResponseJSONShape asserts the wire shape of versionResponse is exactly
// {app_version, build_time, commit_sha, environment, migration_version} and that
// no secret field names are present.
func TestVersionResponseJSONShape(t *testing.T) {
	resp := versionResponse{
		AppVersion:       "0.1.0",
		CommitSHA:        "abc123",
		BuildTime:        "2026-05-10T10:00:00Z",
		Environment:      "development",
		MigrationVersion: 11,
	}
	keys := jsonKeys(t, resp)
	want := []string{"app_version", "build_time", "commit_sha", "environment", "migration_version"}
	assertKeysEqual(t, keys, want)

	// Ensure no credential or internal-routing field slipped in.
	forbidden := []string{
		"token", "secret", "password", "jwt", "dsn", "database_url",
		"db_pass", "db_host", "db_port", "db_user",
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, f := range forbidden {
		if _, found := m[f]; found {
			t.Fatalf("forbidden key %q found in versionResponse JSON", f)
		}
	}
}
