package handlers

// readyResponse is the success body returned by GET /ready.
type readyResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// versionResponse is the body returned by GET /version.
type versionResponse struct {
	AppVersion       string `json:"app_version"`
	CommitSHA        string `json:"commit_sha"`
	BuildTime        string `json:"build_time"`
	Environment      string `json:"environment"`
	MigrationVersion int64  `json:"migration_version"`
}
