package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeHealthRepo implements models.HealthRepository for handler tests.
type fakeHealthRepo struct {
	pingErr       error
	identity      string
	identityErr   error
	migVersion    int64
	migVersionErr error
}

func (f *fakeHealthRepo) Ping(_ context.Context) error                { return f.pingErr }
func (f *fakeHealthRepo) QueryDBIdentity(_ context.Context) (string, error) {
	return f.identity, f.identityErr
}
func (f *fakeHealthRepo) MigrationVersion(_ context.Context) (int64, error) {
	return f.migVersion, f.migVersionErr
}

// --- DTO shape tests ---

// TestReadyResponseJSONShape asserts the top-level wire shape of readyResponse.
func TestReadyResponseJSONShape(t *testing.T) {
	resp := readyResponse{
		Status: "ok",
		Checks: readyChecks{Database: "ok", DatabaseIdentity: "ok"},
	}
	keys := jsonKeys(t, resp)
	assertKeysEqual(t, keys, []string{"checks", "status"})
}

// TestReadyChecksJSONShape asserts the nested checks object shape.
func TestReadyChecksJSONShape(t *testing.T) {
	c := readyChecks{Database: "ok", DatabaseIdentity: "skipped"}
	keys := jsonKeys(t, c)
	assertKeysEqual(t, keys, []string{"database", "database_identity"})
}

// TestVersionResponseJSONShape asserts the wire shape when migration_version is available.
func TestVersionResponseJSONShape(t *testing.T) {
	v := int64(11)
	resp := versionResponse{
		AppVersion:       "0.1.0",
		CommitSHA:        "abc123",
		BuildTime:        "2026-05-10T10:00:00Z",
		Environment:      "development",
		MigrationVersion: &v,
	}
	keys := jsonKeys(t, resp)
	assertKeysEqual(t, keys, []string{"app_version", "build_time", "commit_sha", "environment", "migration_version"})

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

// TestVersionResponseJSONShape_NoMigrationVersion asserts migration_version is omitted when nil.
func TestVersionResponseJSONShape_NoMigrationVersion(t *testing.T) {
	resp := versionResponse{
		AppVersion:  "0.1.0",
		CommitSHA:   "abc123",
		BuildTime:   "2026-05-10T10:00:00Z",
		Environment: "development",
	}
	keys := jsonKeys(t, resp)
	assertKeysEqual(t, keys, []string{"app_version", "build_time", "commit_sha", "environment"})
}

// --- httptest endpoint tests ---

func newTestRouter(h *HealthHandler) *gin.Engine {
	r := gin.New()
	r.GET("/health", h.Live)
	r.GET("/ready", h.Ready)
	r.GET("/version", h.Version)
	return r
}

func TestLive_Returns200(t *testing.T) {
	svc := services.NewHealthService(&fakeHealthRepo{}, config.EnvDevelopment)
	r := newTestRouter(NewHealthHandler(svc, config.EnvDevelopment))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status: got %q want %q", body["status"], "ok")
	}
}

func TestReady_DevEnv_Success(t *testing.T) {
	svc := services.NewHealthService(&fakeHealthRepo{}, config.EnvDevelopment)
	r := newTestRouter(NewHealthHandler(svc, config.EnvDevelopment))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200: body=%s", w.Code, w.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status: got %v", body["status"])
	}
	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatalf("checks field missing or wrong type: %v", body)
	}
	if checks["database"] != "ok" {
		t.Errorf("checks.database: got %v", checks["database"])
	}
	if checks["database_identity"] != "skipped" {
		t.Errorf("checks.database_identity: got %v want skipped", checks["database_identity"])
	}
}

func TestReady_StagingMatch_Success(t *testing.T) {
	svc := services.NewHealthService(&fakeHealthRepo{identity: "staging"}, config.EnvStaging)
	r := newTestRouter(NewHealthHandler(svc, config.EnvStaging))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200: body=%s", w.Code, w.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	checks, _ := body["checks"].(map[string]any)
	if checks["database_identity"] != "ok" {
		t.Errorf("checks.database_identity: got %v want ok", checks["database_identity"])
	}
}

func TestReady_DBUnavailable_Returns503(t *testing.T) {
	svc := services.NewHealthService(
		&fakeHealthRepo{pingErr: errors.New("connection refused")},
		config.EnvDevelopment,
	)
	r := newTestRouter(NewHealthHandler(svc, config.EnvDevelopment))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("error key missing in 503 response")
	}
	// 503 body must not contain internal details.
	raw := w.Body.String()
	for _, forbidden := range []string{"password", "dsn", "postgres://", "connection refused"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("503 body contains forbidden string %q: %s", forbidden, raw)
		}
	}
}

func TestReady_IdentityMismatch_Returns503(t *testing.T) {
	svc := services.NewHealthService(
		&fakeHealthRepo{identity: "production"},
		config.EnvStaging,
	)
	r := newTestRouter(NewHealthHandler(svc, config.EnvStaging))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503", w.Code)
	}
	// The error message must not reveal expected vs. actual identity values.
	raw := w.Body.String()
	for _, forbidden := range []string{"production", "staging"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("503 body leaks environment name %q: %s", forbidden, raw)
		}
	}
}

func TestVersion_WithMigrationVersion(t *testing.T) {
	svc := services.NewHealthService(&fakeHealthRepo{migVersion: 11}, config.EnvDevelopment)
	r := newTestRouter(NewHealthHandler(svc, config.EnvDevelopment))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/version", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"app_version", "commit_sha", "build_time", "environment", "migration_version"} {
		if _, ok := body[field]; !ok {
			t.Errorf("missing field %q in version response", field)
		}
	}
	if body["environment"] != "development" {
		t.Errorf("environment: got %v want development", body["environment"])
	}
	// Verify no secret-like fields are present.
	raw := w.Body.String()
	for _, forbidden := range []string{"password", "secret", "token", "dsn", "db_host", "db_user"} {
		if strings.Contains(raw, `"`+forbidden+`"`) {
			t.Errorf("version response contains forbidden key %q", forbidden)
		}
	}
}

func TestVersion_MigrationVersionUnavailable(t *testing.T) {
	svc := services.NewHealthService(
		&fakeHealthRepo{migVersionErr: errors.New("table does not exist")},
		config.EnvDevelopment,
	)
	r := newTestRouter(NewHealthHandler(svc, config.EnvDevelopment))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/version", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// migration_version must be absent, not falsely reported as 0.
	if _, present := body["migration_version"]; present {
		t.Errorf("migration_version should be absent on query failure, got: %v", body["migration_version"])
	}
	// Endpoint must still return 200 with the other fields.
	for _, field := range []string{"app_version", "commit_sha", "build_time", "environment"} {
		if _, ok := body[field]; !ok {
			t.Errorf("missing field %q in version response", field)
		}
	}
	// DB error text must not appear in the response.
	if strings.Contains(w.Body.String(), "table does not exist") {
		t.Error("version response leaks DB error text")
	}
}
