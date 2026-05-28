// Command setup-firebase-test-users provisions the four test Firebase users
// (TEST_UID_A, TEST_UID_B, TEST_UID_DENIED, TEST_UID_CONFLICT) in the target
// Firebase project with EmailVerified=true. It is idempotent (create-or-update).
//
// Usage:
//
//	FIREBASE_PROJECT_ID=... FIREBASE_CREDENTIALS_FILE=... \
//	  TEST_UID_A=... TEST_UID_B=... TEST_UID_DENIED=... TEST_UID_CONFLICT=... \
//	  setup-firebase-test-users --allow-project-id-file=<path>
//
// Guard checks (in order, before any Admin SDK call):
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

// Synthetic email convention: <role>@test.algoedge.local
const syntheticEmailDomain = "@test.algoedge.local"

// Role labels for the synthetic email addresses — one per test UID.
const (
	roleA        = "a"
	roleB        = "b"
	roleDenied   = "denied"
	roleConflict = "conflict"
)

// Env var keys for the four test UIDs.
const (
	envTestUIDA        = "TEST_UID_A"
	envTestUIDB        = "TEST_UID_B"
	envTestUIDDenied   = "TEST_UID_DENIED"
	envTestUIDConflict = "TEST_UID_CONFLICT"
)

// exitCodeGuardFailed is the exit code returned for any guard violation.
// Named constant per rule 17.
const exitCodeGuardFailed = 2

// fixtureUserClient is the Admin SDK subset needed by setup. Defined as an
// interface so tests inject a fake without hitting Firebase.
type fixtureUserClient interface {
	// CreateOrUpdateUser creates a Firebase user if it does not exist, or
	// updates the existing one. EmailVerified must be set to true on the
	// resulting record.
	CreateOrUpdateUser(ctx context.Context, uid, email string) error
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr, nil))
}

// run is the testable entry point. clientFactory is called after all guards
// pass to obtain a fixtureUserClient. Pass nil to use the real Firebase Admin
// SDK; tests inject a factory that returns a fake.
func run(
	args []string,
	getenv func(string) string,
	stderr *os.File,
	clientFactory func(ctx context.Context, credsFile, projectID string) (fixtureUserClient, error),
) int {
	fs := flag.NewFlagSet("setup-firebase-test-users", flag.ContinueOnError)
	allowFile := fs.String("allow-project-id-file", "", "path to the root-owned guard file containing the expected Firebase project ID (required)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	// Guard 1 — required env vars.
	allKeys := append([]string{"FIREBASE_PROJECT_ID", "FIREBASE_CREDENTIALS_FILE"}, envTestUIDA, envTestUIDB, envTestUIDDenied, envTestUIDConflict)
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
		clientFactory = newRealFirebaseClient
	}
	client, err := clientFactory(ctx, credsFile, projectID)
	if err != nil {
		fmt.Fprintln(stderr, "error: firebase init:", err)
		return 1
	}

	// Provision the four test users.
	users := []struct {
		envKey string
		role   string
	}{
		{envTestUIDA, roleA},
		{envTestUIDB, roleB},
		{envTestUIDDenied, roleDenied},
		{envTestUIDConflict, roleConflict},
	}

	for _, u := range users {
		uid := envVals[u.envKey]
		email := u.role + syntheticEmailDomain
		if err := client.CreateOrUpdateUser(ctx, uid, email); err != nil {
			fmt.Fprintf(stderr, "error: create/update user %s: %v\n", u.envKey, err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "provisioned %s (%s)\n", u.envKey, email)
	}

	fmt.Fprintln(os.Stdout, "setup complete")
	return 0
}

// realFirebaseClient wraps the Firebase Admin SDK auth.Client and satisfies
// fixtureUserClient for production use.
type realFirebaseClient struct {
	auth *firebaseadmin.Client
}

// newRealFirebaseClient initializes the Firebase Admin SDK and returns a
// realFirebaseClient. Called only after all guards have passed.
func newRealFirebaseClient(ctx context.Context, credsFile, projectID string) (fixtureUserClient, error) {
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
	return &realFirebaseClient{auth: authClient}, nil
}

// CreateOrUpdateUser creates the Firebase user with the given uid and email
// (EmailVerified=true) if it does not exist, or updates the existing record if
// it does. This makes setup idempotent.
func (c *realFirebaseClient) CreateOrUpdateUser(ctx context.Context, uid, email string) error {
	params := (&firebaseadmin.UserToCreate{}).
		UID(uid).
		Email(email).
		EmailVerified(true)

	_, err := c.auth.CreateUser(ctx, params)
	if err == nil {
		return nil
	}

	// If the user already exists, update instead.
	if firebaseadmin.IsUserNotFound(err) {
		// User not found during update path — should not happen but handle gracefully.
		return err
	}

	// Try update for any error (the most common case is "user already exists").
	updateParams := (&firebaseadmin.UserToUpdate{}).
		Email(email).
		EmailVerified(true)
	_, updateErr := c.auth.UpdateUser(ctx, uid, updateParams)
	if updateErr != nil {
		return fmt.Errorf("create: %w; update: %w", err, updateErr)
	}
	return nil
}
