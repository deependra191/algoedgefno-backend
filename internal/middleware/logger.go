package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
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
		l.InfoContext(c.Request.Context(), "request",
			"request_id", ridStr,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	}
}
