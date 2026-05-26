package handlers

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// extractUserID retrieves the typed UUID identity set by the auth middleware.
// Returns (uuid.Nil, false) after writing a 401 — caller must just return.
func extractUserID(c *gin.Context) (uuid.UUID, bool) {
	raw, ok := c.Get(middleware.UserIDKey)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user identity"})
		return uuid.Nil, false
	}
	uid, ok := raw.(uuid.UUID)
	if !ok || uid == uuid.Nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing user identity"})
		return uuid.Nil, false
	}
	return uid, true
}
