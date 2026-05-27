package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	bearerPrefix                           = "Bearer "
	appConfigPath                          = "/api/v1/config/app"
	errMissingOrInvalidAuthorizationHeader = "missing or invalid authorization header"
)

// Auth permits the static APP_SECRET_TOKEN on the config app path only; all other paths return 401 until PR 2 reintroduces the JWT path.
func Auth(appSecretToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingOrInvalidAuthorizationHeader})
			return
		}
		token := strings.TrimPrefix(header, bearerPrefix)
		if c.Request.URL.Path == appConfigPath &&
			appSecretToken != "" &&
			subtle.ConstantTimeCompare([]byte(token), []byte(appSecretToken)) == 1 {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingOrInvalidAuthorizationHeader})
	}
}
