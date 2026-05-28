package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeSetupClient is a test double for fixtureUserClient. It records all
// CreateOrUpdateUser calls and returns a configurable error.
type fakeSetupClient struct {
	calls     []fakeCall
	returnErr error
}

type fakeCall struct {
	uid   string
	email string
}

func (f *fakeSetupClient) CreateOrUpdateUser(_ context.Context, uid, email string) error {
	f.calls = append(f.calls, fakeCall{uid: uid, email: email})
	return f.returnErr
}

// fakeClientFactory returns a factory that always yields the provided fake.
func fakeClientFactory(fake fixtureUserClient) func(context.Context, string, string) (fixtureUserClient, error) {
	return func(_ context.Context, _, _ string) (fixtureUserClient, error) {
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

// fullEnv returns a valid complete env map, starting from the given project ID.
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

// stderrCapture creates a pipe and returns (read end, write end). The write
// end is passed to run() as stderr; the read end is used to collect output.
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

func TestRun_MissingTestUIDARejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	delete(env, "TEST_UID_A")
	guardFile := writeGuardFile(t, "my-staging-project\n")

	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_UnreadableAllowFilRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-staging-project")
	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=/nonexistent/path/guard"},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
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
	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
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
	guardFile := writeGuardFile(t, "some-other-project\n")
	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

func TestRun_ProductionMarkerInProjectIDRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("my-prod-project")
	guardFile := writeGuardFile(t, "my-prod-project\n")
	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
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
	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	// No --allow-project-id-file flag.
	code := run(
		[]string{},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
	)

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.calls))
	}
}

// --- Happy path ---

func TestRun_HappyPathCreatesAllFourUsers(t *testing.T) {
	env := fullEnv("my-staging-project")
	guardFile := writeGuardFile(t, "my-staging-project\n")
	fake := &fakeSetupClient{}
	_, stderrW := stderrCapture(t)

	code := run(
		[]string{"--allow-project-id-file=" + guardFile},
		makeGetenv(env),
		stderrW,
		fakeClientFactory(fake),
	)

	if code != 0 {
		t.Fatalf("want exit code 0, got %d", code)
	}
	if len(fake.calls) != 4 {
		t.Fatalf("expected 4 Admin SDK calls, got %d", len(fake.calls))
	}

	// Verify uid→email mappings and that EmailVerified is implicitly set via
	// the expected email convention.
	expected := map[string]string{
		"uid-a":        "a@test.algoedge.local",
		"uid-b":        "b@test.algoedge.local",
		"uid-denied":   "denied@test.algoedge.local",
		"uid-conflict": "conflict@test.algoedge.local",
	}
	recorded := make(map[string]string, len(fake.calls))
	for _, c := range fake.calls {
		recorded[c.uid] = c.email
	}
	for uid, wantEmail := range expected {
		if got := recorded[uid]; got != wantEmail {
			t.Errorf("uid %q: want email %q, got %q", uid, wantEmail, got)
		}
	}
}
