package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestBodyLimit wraps the incoming request body with http.MaxBytesReader so
// that any read past maxBytes returns *http.MaxBytesError. The limit is
// enforced at the I/O layer — before ShouldBindJSON allocates buffers for the
// entire payload — which prevents memory exhaustion from oversized bodies.
//
// The middleware itself does not write any response: http.MaxBytesReader is
// lazy and only triggers when the body is read (inside ShouldBindJSON). The
// handler is responsible for mapping the resulting bind error to an appropriate
// HTTP status (e.g. 400 invalid_request).
func RequestBodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
