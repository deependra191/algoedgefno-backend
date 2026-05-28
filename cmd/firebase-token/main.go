// Command firebase-token mints a Firebase ID token for a given UID by
// performing the two-step custom-token exchange against the Firebase Auth REST
// API. It is an operator tool used by smoke scripts to obtain a real Firebase
// ID token without a browser or Android client.
//
// Usage:
//
//	FIREBASE_CREDENTIALS_FILE=... FIREBASE_PROJECT_ID=... FIREBASE_WEB_API_KEY=... \
//	    firebase-token --uid=<uid>
//
// The ID token is printed to stdout (and nothing else). All errors go to
// stderr. The operator captures stdout into a mode-600 temp file per the
// smoke-script pattern.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

// Env var keys — named per rule 17.
const (
	envFirebaseCredentialsFile = "FIREBASE_CREDENTIALS_FILE"
	envFirebaseProjectID       = "FIREBASE_PROJECT_ID"
	envFirebaseWebAPIKey       = "FIREBASE_WEB_API_KEY"
)

// identitytoolkit REST API constant.
const identitytoolkitSignInURL = "https://identitytoolkit.googleapis.com/v1/accounts:signInWithCustomToken"

// JSON field names for the signInWithCustomToken request and response.
const (
	jsonFieldToken             = "token"
	jsonFieldReturnSecureToken = "returnSecureToken"
	jsonFieldIDToken           = "idToken"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run is the testable entry point. It accepts the CLI args, an env lookup
// function, and output writers, and returns an OS exit code (0 = success,
// non-zero = failure). It never reads os.Args or os.Getenv directly so that
// tests can inject fakes.
func run(args []string, getenv func(string) string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("firebase-token", flag.ContinueOnError)
	uid := fs.String("uid", "", "Firebase UID to mint an ID token for (required)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if strings.TrimSpace(*uid) == "" {
		fmt.Fprintln(stderr, "error: --uid is required")
		return 1
	}

	credsFile := strings.TrimSpace(getenv(envFirebaseCredentialsFile))
	projectID := strings.TrimSpace(getenv(envFirebaseProjectID))
	apiKey := strings.TrimSpace(getenv(envFirebaseWebAPIKey))

	if credsFile == "" {
		fmt.Fprintf(stderr, "error: %s is required\n", envFirebaseCredentialsFile)
		return 1
	}
	if projectID == "" {
		fmt.Fprintf(stderr, "error: %s is required\n", envFirebaseProjectID)
		return 1
	}
	if apiKey == "" {
		fmt.Fprintf(stderr, "error: %s is required\n", envFirebaseWebAPIKey)
		return 1
	}

	idToken, err := mintIDToken(context.Background(), credsFile, projectID, apiKey, strings.TrimSpace(*uid))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	fmt.Fprint(stdout, idToken)
	return 0
}

// mintIDToken performs the two-step exchange:
//  1. Init Firebase Admin SDK and call CustomToken to get a custom token.
//  2. POST to identitytoolkit to exchange the custom token for an ID token.
//
// The custom token and API key are never logged (rule 4). Only the final ID
// token is returned to the caller for printing to stdout.
func mintIDToken(ctx context.Context, credsFile, projectID, apiKey, uid string) (string, error) {
	fbApp, err := firebase.NewApp(ctx,
		&firebase.Config{ProjectID: projectID},
		option.WithCredentialsFile(credsFile),
	)
	if err != nil {
		return "", fmt.Errorf("firebase NewApp: %w", err)
	}

	authClient, err := fbApp.Auth(ctx)
	if err != nil {
		return "", fmt.Errorf("firebase Auth client: %w", err)
	}

	customToken, err := authClient.CustomToken(ctx, uid)
	if err != nil {
		return "", fmt.Errorf("CustomToken: %w", err)
	}

	idToken, err := exchangeCustomToken(ctx, apiKey, customToken)
	if err != nil {
		return "", fmt.Errorf("signInWithCustomToken: %w", err)
	}

	return idToken, nil
}

// exchangeCustomToken POSTs the custom token to the Firebase Auth REST API and
// returns the resulting ID token. The API key and custom token are never
// included in log output (rule 4).
func exchangeCustomToken(ctx context.Context, apiKey, customToken string) (string, error) {
	body, err := json.Marshal(map[string]interface{}{
		jsonFieldToken:             customToken,
		jsonFieldReturnSecureToken: true,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := identitytoolkitSignInURL + "?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP POST: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Include status code but not the raw response body (may contain the API key).
		return "", fmt.Errorf("identitytoolkit returned HTTP %d", resp.StatusCode)
	}

	idTokenRaw, ok := result[jsonFieldIDToken]
	if !ok {
		return "", fmt.Errorf("response missing %q field", jsonFieldIDToken)
	}
	idToken, ok := idTokenRaw.(string)
	if !ok || idToken == "" {
		return "", fmt.Errorf("response %q field is not a non-empty string", jsonFieldIDToken)
	}

	return idToken, nil
}
