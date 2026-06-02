// Command verify-prod-smoke-user marks PROD_SMOKE_UID emailVerified=true in the
// configured production Firebase project. It is an operator tool for enabling
// standard production smoke after the smoke user has been created.
//
// Usage:
//
//	FIREBASE_PROJECT_ID=... FIREBASE_CREDENTIALS_FILE=... PROD_SMOKE_UID=... \
//	  verify-prod-smoke-user
//
// Guard checks run before any Admin SDK call:
//  1. Required env vars are present.
//  2. FIREBASE_PROJECT_ID is the known production project ID.
//  3. PROD_SMOKE_UID has a conservative Firebase UID shape.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	firebaseadmin "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"github.com/deependra191/algoedgefno-backend/internal/firebasefixture"
)

const (
	envFirebaseProjectID       = "FIREBASE_PROJECT_ID"
	envFirebaseCredentialsFile = "FIREBASE_CREDENTIALS_FILE"
	envProdSmokeUID            = "PROD_SMOKE_UID"
)

const (
	exitCodeGuardFailed      = 2
	knownProdFirebaseProject = "algoedgefno"
)

type prodSmokeUserClient interface {
	// MarkEmailVerified sets emailVerified=true for uid.
	MarkEmailVerified(ctx context.Context, uid string) error
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr, nil))
}

func run(
	args []string,
	getenv func(string) string,
	stderr *os.File,
	clientFactory func(ctx context.Context, credsFile, projectID string) (prodSmokeUserClient, error),
) int {
	fs := flag.NewFlagSet("verify-prod-smoke-user", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "error: unexpected positional arguments")
		return exitCodeGuardFailed
	}

	envVals, err := firebasefixture.RequireEnv(getenv, envFirebaseProjectID, envFirebaseCredentialsFile, envProdSmokeUID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	projectID := envVals[envFirebaseProjectID]
	credsFile := envVals[envFirebaseCredentialsFile]
	uid := envVals[envProdSmokeUID]

	if err := requireProductionProject(projectID); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}
	if err := validateFirebaseUID(uid); err != nil {
		fmt.Fprintln(stderr, err)
		return exitCodeGuardFailed
	}

	ctx := context.Background()
	if clientFactory == nil {
		clientFactory = newRealProdSmokeUserClient
	}
	client, err := clientFactory(ctx, credsFile, projectID)
	if err != nil {
		fmt.Fprintln(stderr, "error: firebase init:", err)
		return 1
	}

	if err := client.MarkEmailVerified(ctx, uid); err != nil {
		fmt.Fprintln(stderr, "error: mark PROD_SMOKE_UID email verified:", err)
		return 1
	}

	fmt.Fprintln(os.Stdout, "PROD_SMOKE_UID emailVerified=true")
	return 0
}

func requireProductionProject(projectID string) error {
	if projectID == knownProdFirebaseProject {
		return nil
	}
	return fmt.Errorf("FIREBASE_PROJECT_ID must be the production project %q", knownProdFirebaseProject)
}

func validateFirebaseUID(uid string) error {
	if len(uid) < 6 || len(uid) > 128 {
		return fmt.Errorf("PROD_SMOKE_UID must be 6-128 characters")
	}
	for _, r := range uid {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("PROD_SMOKE_UID contains invalid character")
		}
	}
	return nil
}

type realProdSmokeUserClient struct {
	auth *firebaseadmin.Client
}

func newRealProdSmokeUserClient(ctx context.Context, credsFile, projectID string) (prodSmokeUserClient, error) {
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
	return &realProdSmokeUserClient{auth: authClient}, nil
}

func (c *realProdSmokeUserClient) MarkEmailVerified(ctx context.Context, uid string) error {
	params := (&firebaseadmin.UserToUpdate{}).EmailVerified(true)
	_, err := c.auth.UpdateUser(ctx, uid, params)
	return err
}
