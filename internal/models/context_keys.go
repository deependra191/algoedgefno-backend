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

// LogAttrRequestID is the slog attribute name for the request ID. This is
// the SINGLE canonical definition. The existing constant in
// internal/middleware/logger.go is REMOVED in PR 2; middleware references
// models.LogAttrRequestID instead. Arch-lint already permits middleware →
// models.
const LogAttrRequestID = "request_id"

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
