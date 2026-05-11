package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Request log schema constants. All fields are extracted together so the request
// log is treated as a stable schema — changing one field name is a deliberate,
// visible act rather than an inline edit.
//
// LogAttrRequestID is exported because it is the cross-system correlation key:
// Android clients embed it in crash reports, server-side log queries filter by it,
// and any handler or service that wants to attach the same ID to its own log calls
// needs the name. The remaining constants are internal formatting details.
const (
	// LogAttrRequestID is the slog attribute key for the per-request correlation ID.
	LogAttrRequestID = "request_id"

	logMessageRequest = "request"
	logAttrMethod     = "method"
	logAttrPath       = "path"
	logAttrStatus     = "status"
	logAttrLatencyMS  = "latency_ms"
)

// Logger returns a Gin middleware that emits a structured slog record for each completed request.
// It reads the request ID from the Gin context (set by the RequestID middleware) and logs
// method, path, status, latency_ms, and request_id.
// Authorization headers, tokens, passwords, and DSNs are never logged.
func Logger(l *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		rid, _ := c.Get(RequestIDKey)
		ridStr, _ := rid.(string) // empty string if RequestID middleware is not in the chain
		l.InfoContext(c.Request.Context(), logMessageRequest,
			LogAttrRequestID, ridStr,
			logAttrMethod, c.Request.Method,
			logAttrPath, c.Request.URL.Path,
			logAttrStatus, c.Writer.Status(),
			logAttrLatencyMS, time.Since(start).Milliseconds(),
		)
	}
}
