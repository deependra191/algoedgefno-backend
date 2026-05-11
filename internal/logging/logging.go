// Package logging provides the shared slog handler constructor used by all entry-point
// binaries (cmd/server, cmd/sync). Centralising handler construction prevents the two
// binaries from drifting to different formats or redaction rules.
package logging

import (
	"io"
	"log/slog"
	"strings"

	"github.com/deependra191/algoedgefno-backend/internal/config"
)

// sensitiveKeyParts lists substrings whose presence in an attribute key triggers redaction.
// This is defence-in-depth: code review is the primary control; this catches accidental
// future leaks at the output boundary.
var sensitiveKeyParts = []string{
	"password", "passwd", "secret", "token", "jwt",
	"dsn", "database_url", "credential", "authorization",
}

// NewHandler returns a slog.Handler that writes to w.
// JSON format is used for production and staging (machine-readable); text elsewhere
// (human-readable). A ReplaceAttr hook redacts any attribute whose key contains a
// known-sensitive substring.
func NewHandler(env config.Environment, w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{ReplaceAttr: redactSensitiveKeys}
	if env == config.EnvProduction || env == config.EnvStaging {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

// redactSensitiveKeys replaces the value of any attribute whose key contains a
// known-sensitive substring with "[redacted]".
func redactSensitiveKeys(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(key, part) {
			return slog.Attr{Key: a.Key, Value: slog.StringValue("[redacted]")}
		}
	}
	return a
}
