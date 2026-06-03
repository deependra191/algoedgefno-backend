package handlers

import (
	"errors"
	"net/http"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/gin-gonic/gin"
)

// StrategyHandler handles HTTP requests for built-in strategy listing and detail.
type StrategyHandler struct {
	strategySvc *services.StrategyService
}

// NewStrategyHandler creates a StrategyHandler wired to the strategy service.
func NewStrategyHandler(strategySvc *services.StrategyService) *StrategyHandler {
	return &StrategyHandler{strategySvc: strategySvc}
}

// List returns all strategy sections (BUILTIN + CUSTOM).
func (h *StrategyHandler) List(c *gin.Context) {
	userID := mustUserID(c)
	sections, err := h.strategySvc.ListSections(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list strategies"})
		return
	}
	c.JSON(http.StatusOK, toStrategySectionsResponse(sections))
}

// GetByID returns the full detail for a single strategy by slug.
func (h *StrategyHandler) GetByID(c *gin.Context) {
	userID := mustUserID(c)
	slug := c.Param("id")
	detail, err := h.strategySvc.GetBySlug(c.Request.Context(), slug, userID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "strategy not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get strategy"})
		return
	}
	c.JSON(http.StatusOK, toStrategyDetailResponse(detail))
}
