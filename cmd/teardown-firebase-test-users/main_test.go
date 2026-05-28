package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeTeardownClient is a test double for fixtureDeleteClient. It records all
// DeleteUser calls and returns a configurable error.
type fakeTeardownClient struct {
	calls     []string
	returnErr error
}

func (f *fakeTeardownClient) DeleteUser(_ context.Context, uid string) error {
	f.calls = append(f.calls, uid)
	return f.returnErr
}

// fakeDeleteClientFactory returns a factory that always yields the provided fake.
func fakeDeleteClientFactory(fake fixtureDeleteClient) func(context.Context, string, string) (fixtureDeleteClient, error) {
	return func(_ context.Context, _, _ string) (fixtureDeleteClient, error) {
		return fake, nil
	}
}

// writeGuardFile writes content to a temp file and returns its path.
func writeGuardFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "guard")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write guard file: %v", err)
	}
	return p
}

// fullEnv returns a valid complete env map.
func fullEnv(projectID string) map[string]string {
	return map[string]string{
		"FIREBASE_PROJECT_ID":       projectID,
		"FIREBASE_CREDENTIALS_FILE": "/fake/creds.json",
		"TEST_UID_A":                "uid-a",
		"TEST_UID_B":                "uid-b",
		"TEST_UID_DENIED":           "uid-denied",
		"TEST_UID_CONFLICT":         "uid-conflict",
	}
}

func makeGetenv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// stderrCapture creates a pipe for capturing stderr.
func stderrCapture(t *testing.T) (r *os.File, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r, w
}

// --- Guard tests ---

func TestRun_MissingEnvRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	delete(env, "TEST_UID_A")
	guardFile := writeGuardFile(t, "my-staging-project\n")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_UnreadableAllowFileRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=/nonexistent/path/guard"},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_EmptyAllowFileRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	guardFile := writeGuardFile(t, "")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_AllowFileProjectDiffersFromEnvRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	guardFile := writeGuardFile(t, "different-project\n")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_ProductionMarkerInProjectIDRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("algoedge-production")
	guardFile := writeGuardFile(t, "algoedge-production\n")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_MissingAllowFileFlagRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	// No --allow-project-id-file flag.
	code := run(
		[]string{},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

// --- Happy path ---

func TestRun_HappyPathDeletesAllFourUsers(t *testing.T) {
	env := fullEnv("my-staging-project")
	guardFile := writeGuardFile(t, "my-staging-project\n")
	fake := &fakeTeardownClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeDeleteClientFactory(fake),
	)

	if code != 0 {
		t.Fatalf("want exit code 0, got %d", code)
	}
	if len(fake.calls) != 4 {
		t.Fatalf("expected 4 Admin SDK delete calls, got %d", len(fake.calls))
	}

	wantUIDs := map[string]bool{
		"uid-a":        true,
		"uid-b":        true,
		"uid-denied":   true,
		"uid-conflict": true,
	}
	for _, uid := range fake.calls {
		if !wantUIDs[uid] {
			t.Errorf("unexpected UID deleted: %q", uid)
		}
		delete(wantUIDs, uid)
	}
	for uid := range wantUIDs {
		t.Errorf("expected UID %q to be deleted, but it was not", uid)
	}
}
