package firebasefixture_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/firebasefixture"
)

// --- RequireEnv ---

func TestRequireEnv_AllPresent(t *testing.T) {
	getenv := func(k string) string {
		m := map[string]string{"A": "val-a", "B": "val-b"}
		return m[k]
	}
	vals, err := firebasefixture.RequireEnv(getenv, "A", "B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals["A"] != "val-a" || vals["B"] != "val-b" {
		t.Fatalf("unexpected values: %v", vals)
	}
}

func TestRequireEnv_MissingKey(t *testing.T) {
	getenv := func(k string) string {
		if k == "PRESENT" {
			return "yes"
		}
		return ""
	}
	_, err := firebasefixture.RequireEnv(getenv, "PRESENT", "MISSING_VAR")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
	want := "missing env: MISSING_VAR"
	if err.Error() != want {
		t.Fatalf("want %q, got %q", want, err.Error())
	}
}

func TestRequireEnv_BlankValue(t *testing.T) {
	getenv := func(k string) string {
		if k == "BLANK" {
			return "   "
		}
		return ""
	}
	_, err := firebasefixture.RequireEnv(getenv, "BLANK")
	if err == nil {
		t.Fatal("expected error for blank value, got nil")
	}
}

func TestRequireEnv_FirstMissingReported(t *testing.T) {
	getenv := func(k string) string { return "" }
	_, err := firebasefixture.RequireEnv(getenv, "KEY_ONE", "KEY_TWO")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "missing env: KEY_ONE"
	if err.Error() != want {
		t.Fatalf("want %q, got %q", want, err.Error())
	}
}

// --- CheckAllowProjectIDFile ---

func writeGuardFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "guard")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write guard file: %v", err)
	}
	return p
}

func TestCheckAllowProjectIDFile_HappyPath(t *testing.T) {
	path := writeGuardFile(t, "my-staging-project\n")
	if err := firebasefixture.CheckAllowProjectIDFile(path, "my-staging-project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckAllowProjectIDFile_HappyPathNoTrailingNewline(t *testing.T) {
	path := writeGuardFile(t, "my-staging-project")
	if err := firebasefixture.CheckAllowProjectIDFile(path, "my-staging-project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckAllowProjectIDFile_EmptyFile(t *testing.T) {
	path := writeGuardFile(t, "")
	err := firebasefixture.CheckAllowProjectIDFile(path, "my-project")
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

func TestCheckAllowProjectIDFile_WhitespaceOnlyFile(t *testing.T) {
	path := writeGuardFile(t, "   \n  \n")
	err := firebasefixture.CheckAllowProjectIDFile(path, "my-project")
	if err == nil {
		t.Fatal("expected error for whitespace-only file, got nil")
	}
}

func TestCheckAllowProjectIDFile_MultipleLines(t *testing.T) {
	path := writeGuardFile(t, "project-a\nproject-b\n")
	err := firebasefixture.CheckAllowProjectIDFile(path, "project-a")
	if err == nil {
		t.Fatal("expected error for multi-line file, got nil")
	}
}

func TestCheckAllowProjectIDFile_Mismatch(t *testing.T) {
	path := writeGuardFile(t, "staging-project\n")
	err := firebasefixture.CheckAllowProjectIDFile(path, "different-project")
	if err == nil {
		t.Fatal("expected error for mismatched project ID, got nil")
	}
}

func TestCheckAllowProjectIDFile_UnreadablePath(t *testing.T) {
	err := firebasefixture.CheckAllowProjectIDFile("/nonexistent/path/to/guard", "any")
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
}

// --- RejectProductionMarker ---

func TestRejectProductionMarker_SafeProject(t *testing.T) {
	for _, id := range []string{"my-staging-project", "algoedge-staging-12345", "test-firebase-12"} {
		if err := firebasefixture.RejectProductionMarker(id); err != nil {
			t.Errorf("expected no error for %q, got %v", id, err)
		}
	}
}

func TestRejectProductionMarker_ContainsProd(t *testing.T) {
	for _, id := range []string{"my-prod-project", "prod-algoedge", "algoedge-prod"} {
		if err := firebasefixture.RejectProductionMarker(id); err == nil {
			t.Errorf("expected error for %q, got nil", id)
		}
	}
}

func TestRejectProductionMarker_ContainsProduction(t *testing.T) {
	for _, id := range []string{"my-production-project", "production-algoedge"} {
		if err := firebasefixture.RejectProductionMarker(id); err == nil {
			t.Errorf("expected error for %q, got nil", id)
		}
	}
}

func TestRejectProductionMarker_ContainsLive(t *testing.T) {
	for _, id := range []string{"my-live-project", "live-algoedge", "algoedge-live"} {
		if err := firebasefixture.RejectProductionMarker(id); err == nil {
			t.Errorf("expected error for %q, got nil", id)
		}
	}
}

func TestRejectProductionMarker_CaseInsensitive(t *testing.T) {
	for _, id := range []string{"My-PROD-Project", "PRODUCTION-APP", "LIVE-APP"} {
		if err := firebasefixture.RejectProductionMarker(id); err == nil {
			t.Errorf("expected error for %q (case-insensitive check), got nil", id)
		}
	}
}
