package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const errMissingUserIdentity = "missing user identity"

// RequireUserIdentity returns a Gin middleware that enforces a valid tenant
// identity on the request.
//
// It must run AFTER Auth, which stores the authenticated user's UUID under
// models.UserIDKey. RequireUserIdentity requires that key to be present, of
// type uuid.UUID, and not uuid.Nil; otherwise it aborts with 401 and body
// {"error":"missing user identity"}. On success the request proceeds with the
// route-level invariant that every downstream tenant handler has a valid owner.
func RequireUserIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := c.Get(models.UserIDKey)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingUserIdentity})
			return
		}
		uid, ok := raw.(uuid.UUID)
		if !ok || uid == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingUserIdentity})
			return
		}
		c.Next()
	}
}
