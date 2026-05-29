// Command teardown-firebase-test-users deletes the four test Firebase users
// (TEST_UID_A, TEST_UID_B, TEST_UID_DENIED, TEST_UID_CONFLICT) from the target
// Firebase project. It is idempotent — a missing user is not an error.
//
// Usage:
//
//	FIREBASE_PROJECT_ID=... FIREBASE_CREDENTIALS_FILE=... \
//	  TEST_UID_A=... TEST_UID_B=... TEST_UID_DENIED=... TEST_UID_CONFLICT=... \
//	  teardown-firebase-test-users --allow-project-id-file=<path>
//
// Guard checks (identical to setup, applied in the same order before any
// Admin SDK call):
//  1. All required env vars present — exit 2 on the first missing one.
//  2. --allow-project-id-file must be readable, contain exactly one non-empty
//     line, and that line must equal FIREBASE_PROJECT_ID — exit 2 otherwise.
//  3. FIREBASE_PROJECT_ID must not contain a production marker — exit 2 otherwise.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	firebaseadmin "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"github.com/deependra191/algoedgefno-backend/internal/firebasefixture"
)

// exitCodeGuardFailed is the exit code returned for any guard violation.
const exitCodeGuardFailed = 2

// Env var keys for the four test UIDs — must match setup.
const (
	envTestUIDA        = "TEST_UID_A"
	envTestUIDB        = "TEST_UID_B"
	envTestUIDDenied   = "TEST_UID_DENIED"
	envTestUIDConflict = "TEST_UID_CONFLICT"
)

// fixtureDeleteClient is the Admin SDK subset needed by teardown. Defined as
// an interface so tests inject a fake without hitting Firebase.
type fixtureDeleteClient interface {
	// DeleteUser removes a Firebase user by UID. A missing user must be
	// treated as a success (idempotent).
	DeleteUser(ctx context.Context, uid string) error
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr, nil))
}

// run is the testable entry point. clientFactory is called after all guards
// pass to obtain a fixtureDeleteClient. Pass nil to use the real Firebase Admin
// SDK; tests inject a factory that returns a fake.
func run(
	args []string,
	getenv func(string) string,
	stderr *os.File,
	clientFactory func(ctx context.Context, credsFile, projectID string) (fixtureDeleteClient, error),
) int {
	fs := flag.NewFlagSet("teardown-firebase-test-users", flag.ContinueOnError)
	allowFile := fs.String("allow-project-id-file", "", "path to the root-owned guard file containing the expected Firebase project ID (required)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	// Guard 1 — required env vars.
	allKeys := []string{"FIREBASE_PROJECT_ID", "FIREBASE_CREDENTIALS_FILE", envTestUIDA, envTestUIDB, envTestUIDDenied, envTestUIDConflict}
	envVals, err := firebasefixture.RequireEnv(getenv, allKeys...)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	projectID := envVals["FIREBASE_PROJECT_ID"]
	credsFile := envVals["FIREBASE_CREDENTIALS_FILE"]

	// Guard 2 — allow-project-id-file flag.
	if strings.TrimSpace(*allowFile) == "" {
		fmt.Fprintln(stderr, "error: --allow-project-id-file is required")
		return exitCodeGuardFailed
	}
	if err := firebasefixture.CheckAllowProjectIDFile(*allowFile, projectID); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	// Guard 3 — production-marker reject.
	if err := firebasefixture.RejectProductionMarker(projectID); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	// All guards passed — init the Admin SDK (or the injected fake).
	ctx := context.Background()
	if clientFactory == nil {
		clientFactory = newRealFirebaseDeleteClient
	}
	client, err := clientFactory(ctx, credsFile, projectID)
	if err != nil {
		fmt.Fprintln(stderr, "error: firebase init:", err)
		return 1
	}

	// Delete the four test users.
	uidEnvKeys := []string{envTestUIDA, envTestUIDB, envTestUIDDenied, envTestUIDConflict}
	for _, k := range uidEnvKeys {
		uid := envVals[k]
		if err := client.DeleteUser(ctx, uid); err != nil {
			fmt.Fprintf(stderr, "error: delete user %s: %v\n", k, err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "deleted %s\n", k)
	}

	fmt.Fprintln(os.Stdout, "teardown complete")
	return 0
}

// realFirebaseDeleteClient wraps the Firebase Admin SDK auth.Client for
// deletions and satisfies fixtureDeleteClient.
type realFirebaseDeleteClient struct {
	auth *firebaseadmin.Client
}

// newRealFirebaseDeleteClient initializes the Firebase Admin SDK and returns a
// realFirebaseDeleteClient. Called only after all guards have passed.
func newRealFirebaseDeleteClient(ctx context.Context, credsFile, projectID string) (fixtureDeleteClient, error) {
	fbApp, err := firebase.NewApp(ctx,
		&firebase.Config{ProjectID: projectID},
		option.WithCredentialsFile(credsFile),
	)
	if err != nil {
		return nil, fmt.Errorf("firebase NewApp: %w", err)
	}
	authClient, err := fbApp.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase Auth client: %w", err)
	}
	return &realFirebaseDeleteClient{auth: authClient}, nil
}

// DeleteUser removes the Firebase user with the given UID. A user-not-found
// error is silently ignored to make teardown idempotent.
func (c *realFirebaseDeleteClient) DeleteUser(ctx context.Context, uid string) error {
	err := c.auth.DeleteUser(ctx, uid)
	if err == nil || firebaseadmin.IsUserNotFound(err) {
		return nil
	}
	return err
}
