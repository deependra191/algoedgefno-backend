package models

import "context"

// UserIDKey is the gin.Context key under which an authenticated user's
// uuid.UUID identity is stored. Middleware sets it (PR 2 onward); handlers
// read it via the extractUserID helper. Lives in models so both layers
// reference the same constant without cross-component import.
const UserIDKey = "userID"

// requestIDCtxKey is a private typed key used as the context.Context lookup
// key for the per-request correlation ID. Typed (not string) so it never
// collides with arbitrary string keys in third-party libraries. Private to
// models so the only access is via the exported helpers below.
type requestIDCtxKey struct{}

// Structured-log attribute KEYS — the canonical telemetry schema, defined once
// and shared across layers (the keys you grep for across all logs). Attribute
// VALUES (e.g. a specific event name like "identity_conflict") are
// domain-specific and stay local to the package that emits them.
//
// LogAttrRequestID is the SINGLE canonical definition for the request-ID key;
// the former constant in internal/middleware/logger.go was REMOVED in PR 2 in
// favour of this one. Arch-lint already permits middleware → models.
const (
	LogAttrRequestID = "request_id"
	LogAttrEvent     = "event"
	LogAttrReason    = "reason"
)

// WithRequestID returns ctx augmented with rid. Middleware calls this; the
// returned context is what flows into services.
func WithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey{}, rid)
}

// RequestIDFrom returns the request ID stored in ctx, or "" if none is set.
// Services call this to attach the correlation ID to log records.
func RequestIDFrom(ctx context.Context) string {
	rid, _ := ctx.Value(requestIDCtxKey{}).(string)
	return rid
}
