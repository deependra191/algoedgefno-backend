package handlers

import (
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/gin-gonic/gin"
)

// mustUserID returns the authenticated tenant's UUID from the Gin context.
//
// It is an invariant helper, not a validator: middleware.RequireUserIdentity
// guarantees models.UserIDKey is present, a uuid.UUID, and non-nil before any
// tenant handler runs, so this never yields uuid.Nil on a correctly wired
// route. It panics (recovered as 500 by gin.Recovery) if a tenant route was
// registered without that middleware — surfacing the misconfiguration loudly
// rather than silently querying under uuid.Nil.
func mustUserID(c *gin.Context) uuid.UUID {
	return c.MustGet(models.UserIDKey).(uuid.UUID)
}
