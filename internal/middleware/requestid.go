package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	// RequestIDKey is the Gin context key under which the request ID string is stored.
	RequestIDKey = "requestID"
	// RequestIDHeader is the HTTP header used to carry the request ID in both directions.
	RequestIDHeader = "X-Request-ID"
)

// RequestID accepts a well-formed UUID (any version) in the X-Request-ID request header
// and reuses it, or generates a new UUID v4 if the header is absent or not a valid UUID.
// The ID is stored in the Gin context under RequestIDKey and echoed in the
// X-Request-ID response header so Android clients can correlate requests with server logs.
//
// Trust model: the inbound header is accepted as-is from authenticated Android clients.
// v1 is a single-user app; the header cannot be spoofed without also holding the bearer
// token. If multi-tenant support is added, revisit whether the inbound ID should be
// logged under a separate "client_request_id" key with a distinct server-generated ID.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.GetHeader(RequestIDHeader))
		if err != nil {
			id = uuid.New()
		}
		idStr := id.String()
		c.Set(RequestIDKey, idStr)                                                              // existing — Gin string key
		c.Request = c.Request.WithContext(models.WithRequestID(c.Request.Context(), idStr)) // new — Go context bridge
		c.Header(RequestIDHeader, idStr)
		c.Next()
	}
}
