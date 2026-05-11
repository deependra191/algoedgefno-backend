package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Request log schema constants. All fields are extracted together so the request
// log is treated as a stable schema — changing one field name is a deliberate,
// visible act rather than an inline edit. request_id is also the cross-system
// correlation key used by Android clients and server-side log queries.
const (
	logMessageRequest = "request"

	logAttrRequestID = "request_id"
	logAttrMethod    = "method"
	logAttrPath      = "path"
	logAttrStatus    = "status"
	logAttrLatencyMS = "latency_ms"
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
			logAttrRequestID, ridStr,
			logAttrMethod, c.Request.Method,
			logAttrPath, c.Request.URL.Path,
			logAttrStatus, c.Writer.Status(),
			logAttrLatencyMS, time.Since(start).Milliseconds(),
		)
	}
}
