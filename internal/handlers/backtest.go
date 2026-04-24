package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/gin-gonic/gin"
)

// BacktestHandler handles HTTP requests for backtest submission and retrieval.
type BacktestHandler struct {
	backtestSvc *services.BacktestService
}

// NewBacktestHandler creates a BacktestHandler wired to the backtest service.
func NewBacktestHandler(backtestSvc *services.BacktestService) *BacktestHandler {
	return &BacktestHandler{backtestSvc: backtestSvc}
}

// Submit runs a backtest for a built-in strategy with user-supplied inputs.
func (h *BacktestHandler) Submit(c *gin.Context) {
	var req backtestSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	from, err := time.Parse(dateFormat, req.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date format, expected YYYY-MM-DD"})
		return
	}
	to, err := time.Parse(dateFormat, req.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date format, expected YYYY-MM-DD"})
		return
	}

	if !from.Before(to) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from date must be before to date"})
		return
	}

	run, err := h.backtestSvc.Submit(c.Request.Context(), services.BacktestRequest{
		StrategySlug: req.StrategyID,
		Underlying:   req.Underlying,
		From:         from,
		To:           to,
		Lots:         req.Lots,
		Capital:      req.Capital,
	})
	if err != nil {
		switch err.Error() {
		case "strategy not found", "no instrument found for underlying":
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case "no candle data available":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "backtest failed"})
		}
		return
	}

	c.JSON(http.StatusCreated, toBacktestResultResponse(run))
}

// GetByID returns a completed backtest run by its UUID.
func (h *BacktestHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backtest ID"})
		return
	}

	run, err := h.backtestSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "backtest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get backtest"})
		return
	}

	c.JSON(http.StatusOK, toBacktestResultResponse(run))
}
