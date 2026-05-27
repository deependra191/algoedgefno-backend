package handlers

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/gin-gonic/gin"
)

const errMissingUserIdentity = "missing user identity"

// extractUserID retrieves the typed UUID identity set by the auth middleware.
// Returns (uuid.Nil, false) after writing a 401 — caller must just return.
func extractUserID(c *gin.Context) (uuid.UUID, bool) {
	raw, ok := c.Get(models.UserIDKey)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingUserIdentity})
		return uuid.Nil, false
	}
	uid, ok := raw.(uuid.UUID)
	if !ok || uid == uuid.Nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMissingUserIdentity})
		return uuid.Nil, false
	}
	return uid, true
}
