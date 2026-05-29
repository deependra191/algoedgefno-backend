// Package firebasefixture provides security guard functions shared by
// cmd/setup-firebase-test-users and cmd/teardown-firebase-test-users.
// It imports only stdlib so it is a leaf package with no internal dependencies,
// making it trivially unit-testable without any Firebase SDK or network access.
package firebasefixture

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Production markers that indicate a project ID must be rejected.
const (
	markerProd       = "prod"
	markerProduction = "production"
	markerLive       = "live"
)

// EnvVarRequiredKeys lists the names of the env vars that both setup and
// teardown binaries require. Exported so that callers can document them.
var EnvVarRequiredKeys = []string{
	"FIREBASE_PROJECT_ID",
	"FIREBASE_CREDENTIALS_FILE",
	"TEST_UID_A",
	"TEST_UID_B",
	"TEST_UID_DENIED",
	"TEST_UID_CONFLICT",
}

// RequireEnv resolves all requested keys via getenv. It returns a map of
// key→value on success. On the first missing or blank value it returns an
// error of the form "missing env: <KEY>" so callers can print that message to
// stderr and exit with code 2 without issuing any Firebase API call.
func RequireEnv(getenv func(string) string, keys ...string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(getenv(k))
		if v == "" {
			return nil, fmt.Errorf("missing env: %s", k)
		}
		values[k] = v
	}
	return values, nil
}

// CheckAllowProjectIDFile reads the file at path and expects exactly one
// trimmed non-empty line. It returns an error if:
//   - the file cannot be opened or read,
//   - the file is empty or contains only whitespace,
//   - the file contains more than one non-empty line,
//   - the single line does not equal expectedProjectID.
//
// This guard implements the root-owned authorization boundary: the file is
// provisioned independently by the operator and is the sole authorization
// input for fixture mutation.
func CheckAllowProjectIDFile(path, expectedProjectID string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read allow-project-id-file: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("cannot read allow-project-id-file: %w", err)
	}

	if len(lines) == 0 {
		return fmt.Errorf("allow-project-id-file is empty")
	}
	if len(lines) > 1 {
		return fmt.Errorf("allow-project-id-file must contain exactly one line, found %d", len(lines))
	}

	fileProjectID := lines[0]
	if fileProjectID != expectedProjectID {
		return fmt.Errorf("allow-project-id-file project ID does not match FIREBASE_PROJECT_ID")
	}
	return nil
}

// RejectProductionMarker returns an error if projectID contains any
// case-insensitive production marker (prod, production, live). This is a
// defense-in-depth check — the operator's allow-file guard is the primary
// authorization boundary; this marker check adds a second independent layer.
func RejectProductionMarker(projectID string) error {
	lower := strings.ToLower(projectID)
	for _, marker := range []string{markerProd, markerProduction, markerLive} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("FIREBASE_PROJECT_ID contains production marker %q — fixture binaries must not run against production", marker)
		}
	}
	return nil
}
