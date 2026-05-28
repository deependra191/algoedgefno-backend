package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// newStrategyListRouter returns a gin engine with a single /api/v1/strategies GET
// route that applies setKey then calls the real extractUserID helper.
func newStrategyListRouter(setKey func(c *gin.Context)) *gin.Engine {
	r := gin.New()
	r.GET("/api/v1/strategies", func(c *gin.Context) {
		setKey(c)
		_, ok := extractUserID(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// TestStrategyHandler_MissingIdentity asserts that a missing UserIDKey causes 401 (row 7).
func TestStrategyHandler_MissingIdentity(t *testing.T) {
	r := newStrategyListRouter(func(_ *gin.Context) {
		// Key not set.
	})
	assertMissingUserIdentity(t, doGetJSON(r, "/api/v1/strategies"))
}

// TestStrategyHandler_StringIdentity asserts that a string value for UserIDKey causes 401 (row 8).
func TestStrategyHandler_StringIdentity(t *testing.T) {
	r := newStrategyListRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, "not-a-uuid")
	})
	assertMissingUserIdentity(t, doGetJSON(r, "/api/v1/strategies"))
}

// TestStrategyHandler_NilUUIDIdentity asserts that uuid.Nil as the UserIDKey value causes 401 (row 9).
func TestStrategyHandler_NilUUIDIdentity(t *testing.T) {
	r := newStrategyListRouter(func(c *gin.Context) {
		c.Set(models.UserIDKey, uuid.Nil)
	})
	assertMissingUserIdentity(t, doGetJSON(r, "/api/v1/strategies"))
}
