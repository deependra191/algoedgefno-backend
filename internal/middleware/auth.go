package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	UserIDKey    = "userID"
	bearerPrefix = "Bearer "
)

// Auth permits the static APP_SECRET_TOKEN on "/api/v1/config/app" only; all other paths return 401 until PR 2 reintroduces the JWT path.
func Auth(appSecretToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}
		token := strings.TrimPrefix(header, bearerPrefix)
		if c.Request.URL.Path == "/api/v1/config/app" &&
			appSecretToken != "" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(appSecretToken)) == 1 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
	}
}
