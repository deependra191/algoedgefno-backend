package main

import (
	"context"
	"errors"
	"os"
	"testing"
)

type fakeProdSmokeUserClient struct {
	uids      []string
	returnErr error
}

func (f *fakeProdSmokeUserClient) MarkEmailVerified(_ context.Context, uid string) error {
	f.uids = append(f.uids, uid)
	return f.returnErr
}

func fakeClientFactory(fake prodSmokeUserClient) func(context.Context, string, string) (prodSmokeUserClient, error) {
	return func(_ context.Context, _, _ string) (prodSmokeUserClient, error) {
		return fake, nil
	}
}

func fullEnv(projectID string) map[string]string {
	return map[string]string{
		envFirebaseProjectID:       projectID,
		envFirebaseCredentialsFile: "/run/secrets/firebase-serviceaccount-prod.json",
		envProdSmokeUID:            "3wvHesrFhTNqXDCq11irroKBdw43",
	}
}

func makeGetenv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func stderrCapture(t *testing.T) (r *os.File, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r, w
}

func TestRun_HappyPathMarksProdSmokeUIDVerified(t *testing.T) {
	env := fullEnv(knownProdFirebaseProject)
	fake := &fakeProdSmokeUserClient{}
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != 0 {
		t.Fatalf("want exit code 0, got %d", code)
	}
	if len(fake.uids) != 1 {
		t.Fatalf("expected one Admin SDK call, got %d", len(fake.uids))
	}
	if fake.uids[0] != env[envProdSmokeUID] {
		t.Fatalf("uid = %q, want %q", fake.uids[0], env[envProdSmokeUID])
	}
}

func TestRun_MissingProdSmokeUIDRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv(knownProdFirebaseProject)
	delete(env, envProdSmokeUID)
	fake := &fakeProdSmokeUserClient{}
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.uids) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.uids))
	}
}

func TestRun_StagingProjectRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("algoedgefno-staging")
	fake := &fakeProdSmokeUserClient{}
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.uids) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.uids))
	}
}

func TestRun_ProductionMarkerProjectRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv("algoedgefno-prod")
	fake := &fakeProdSmokeUserClient{}
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.uids) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.uids))
	}
}

func TestRun_InvalidUIDRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv(knownProdFirebaseProject)
	env[envProdSmokeUID] = "bad uid"
	fake := &fakeProdSmokeUserClient{}
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.uids) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.uids))
	}
}

func TestRun_UnexpectedArgRejectedBeforeSDKCall(t *testing.T) {
	env := fullEnv(knownProdFirebaseProject)
	fake := &fakeProdSmokeUserClient{}
	_, stderrW := stderrCapture(t)

	code := run([]string{"unexpected"}, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != exitCodeGuardFailed {
		t.Fatalf("want exit code %d, got %d", exitCodeGuardFailed, code)
	}
	if len(fake.uids) != 0 {
		t.Fatalf("expected zero Admin SDK calls, got %d", len(fake.uids))
	}
}

func TestRun_ClientFactoryErrorReturnsFailure(t *testing.T) {
	env := fullEnv(knownProdFirebaseProject)
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, func(context.Context, string, string) (prodSmokeUserClient, error) {
		return nil, errors.New("boom")
	})

	if code != 1 {
		t.Fatalf("want exit code 1, got %d", code)
	}
}

func TestRun_UpdateErrorReturnsFailure(t *testing.T) {
	env := fullEnv(knownProdFirebaseProject)
	fake := &fakeProdSmokeUserClient{returnErr: errors.New("update failed")}
	_, stderrW := stderrCapture(t)

	code := run(nil, makeGetenv(env), stderrW, fakeClientFactory(fake))

	if code != 1 {
		t.Fatalf("want exit code 1, got %d", code)
	}
	if len(fake.uids) != 1 {
		t.Fatalf("expected one Admin SDK call, got %d", len(fake.uids))
	}
}
