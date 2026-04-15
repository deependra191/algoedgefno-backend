package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/gin-gonic/gin"
)

const (
	UserIDKey    = "userID"
	bearerPrefix = "Bearer "
	appOwnerID   = "app-owner"
)

// Auth checks static bearer token first (v1), then JWT (future multi-user).
func Auth(appSecretToken string, authSvc *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}

		token := strings.TrimPrefix(header, bearerPrefix)

		// Path 1: static app token (v1 single-user)
		if appSecretToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(appSecretToken)) == 1 {
			c.Set(UserIDKey, appOwnerID)
			c.Next()
			return
		}

		// Path 2: JWT (future multi-user)
		userID, err := authSvc.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(UserIDKey, userID)
		c.Next()
	}
}
