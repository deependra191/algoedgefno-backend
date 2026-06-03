package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	bearerPrefix = "Bearer "

	errMissingOrInvalidAuthorizationHeader = "missing or invalid authorization header"
	errInvalidOrExpiredToken               = "invalid or expired token"
)

// Auth returns a Gin middleware that enforces backend JWT authentication.
//
// Requests go through validator.ValidateToken, which parses the HS256 token,
// verifies the env claim, and returns the user UUID. On success the UUID is
// stored in the Gin context under models.UserIDKey.
func Auth(validator models.TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingOrInvalidAuthorizationHeader})
			return
		}
		token := strings.TrimPrefix(header, bearerPrefix)

		uid, err := validator.ValidateToken(token)
		if err != nil || uid == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errInvalidOrExpiredToken})
			return
		}
		c.Set(models.UserIDKey, uid) // uuid.UUID, enforced by RequireUserIdentity on tenant routes
		c.Next()
	}
}
