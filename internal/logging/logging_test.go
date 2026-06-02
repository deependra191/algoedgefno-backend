package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/logging"
)

func TestNewHandler_ProductionUsesJSON(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewHandler(config.EnvProduction, &buf)
	slog.New(h).Info("hello")
	if !strings.HasPrefix(buf.String(), "{") {
		t.Errorf("production handler should emit JSON, got: %s", buf.String())
	}
}

func TestNewHandler_DevelopmentUsesText(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewHandler(config.EnvDevelopment, &buf)
	slog.New(h).Info("hello")
	if strings.HasPrefix(buf.String(), "{") {
		t.Errorf("development handler should emit text, got: %s", buf.String())
	}
}

func TestNewHandler_StagingUsesJSON(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewHandler(config.EnvStaging, &buf)
	slog.New(h).Info("hello")
	if !strings.HasPrefix(buf.String(), "{") {
		t.Errorf("staging handler should emit JSON, got: %s", buf.String())
	}
}

// TestNewHandler_RedactsSensitiveKeys asserts that attribute values for known-sensitive
// keys are replaced with "[redacted]" and the original value is never emitted.
func TestNewHandler_RedactsSensitiveKeys(t *testing.T) {
	cases := []struct {
		key   string
		value string
	}{
		{"password", "hunter2"},
		{"db_password", "secret123"},
		{"jwt_token", "eyJhbGc..."},
		{"authorization", "Bearer tok"},
		{"dsn", "postgres://u:p@host/db"},
		{"database_url", "postgres://u:p@host/db"},
		{"jwt_secret", "jwtkey"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			var buf bytes.Buffer
			h := logging.NewHandler(config.EnvProduction, &buf)
			slog.New(h).Info("test", tc.key, tc.value)
			out := buf.String()
			if strings.Contains(out, tc.value) {
				t.Errorf("sensitive value %q leaked for key %q: %s", tc.value, tc.key, out)
			}
			if !strings.Contains(out, logging.RedactedLogValue) {
				t.Errorf("expected [redacted] for key %q, got: %s", tc.key, out)
			}
		})
	}
}

// TestNewHandler_SafeKeysNotRedacted asserts that non-sensitive keys pass through unchanged.
func TestNewHandler_SafeKeysNotRedacted(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewHandler(config.EnvProduction, &buf)
	slog.New(h).Info("test", "env", "production", "status", 200)
	out := buf.String()
	if strings.Contains(out, logging.RedactedLogValue) {
		t.Errorf("safe keys were unexpectedly redacted: %s", out)
	}
}
