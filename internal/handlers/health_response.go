package handlers

// readyChecks holds the per-check results within a successful readiness response.
type readyChecks struct {
	Database         string `json:"database"`
	DatabaseIdentity string `json:"database_identity"`
}

// readyResponse is the success body returned by GET /ready.
type readyResponse struct {
	Status string      `json:"status"`
	Checks readyChecks `json:"checks"`
}

// versionResponse is the body returned by GET /version.
// MigrationVersion is omitted from the response when the DB query fails.
type versionResponse struct {
	AppVersion       string `json:"app_version"`
	CommitSHA        string `json:"commit_sha"`
	BuildTime        string `json:"build_time"`
	Environment      string `json:"environment"`
	MigrationVersion *int64 `json:"migration_version,omitempty"`
}
