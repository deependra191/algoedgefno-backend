package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

const (
	bearerPrefix  = "Bearer "
	appConfigPath = "/api/v1/config/app"

	errMissingOrInvalidAuthorizationHeader = "missing or invalid authorization header"
	errInvalidOrExpiredToken               = "invalid or expired token"
)

// Auth returns a Gin middleware that enforces bearer-token authentication.
//
// Static-token path: requests to /api/v1/config/app with a token that
// constant-time-matches appSecretToken are allowed without involving the JWT
// validator. This path is reserved for the Android app-config endpoint.
//
// JWT path: all other requests go through validator.ValidateToken, which
// parses the HS256 token, verifies the env claim, and returns the user UUID.
// On success the UUID is stored in the Gin context under models.UserIDKey.
func Auth(appSecretToken string, validator models.TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingOrInvalidAuthorizationHeader})
			return
		}
		token := strings.TrimPrefix(header, bearerPrefix)

		// Static-token path — only for /api/v1/config/app.
		if c.Request.URL.Path == appConfigPath &&
			appSecretToken != "" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(appSecretToken)) == 1 {
			c.Next()
			return
		}

		// JWT validator path — all other protected endpoints.
		uid, err := validator.ValidateToken(token)
		if err != nil || uid == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errInvalidOrExpiredToken})
			return
		}
		c.Set(models.UserIDKey, uid) // uuid.UUID, consumed by extractUserID in handlers
		c.Next()
	}
}
