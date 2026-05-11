package handlers

import (
	"errors"
	"net/http"
	"strconv"
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

// Submit validates the request, creates a run, and fires the engine asynchronously.
// Returns HTTP 202 with {id, status} immediately; client polls GET /backtests/:id.
func (h *BacktestHandler) Submit(c *gin.Context) {
	var req backtestSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	from, err := time.Parse(dateFormat, req.Inputs.DateRange.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date format, expected YYYY-MM-DD"})
		return
	}
	to, err := time.Parse(dateFormat, req.Inputs.DateRange.To)
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
		Underlying:   req.Inputs.Underlying,
		From:         from,
		To:           to,
		Lots:         req.Inputs.Lots,
		Capital:      req.Inputs.Capital,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBacktestDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "backtests are disabled"})
		case errors.Is(err, services.ErrBacktestDateRangeExceeded):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "date range exceeds maximum allowed"})
		case errors.Is(err, services.ErrBacktestCandleCountExceeded):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "candle count exceeds maximum allowed"})
		case errors.Is(err, services.ErrStrategyNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "strategy not found"})
		case errors.Is(err, services.ErrInvalidUnderlying):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid underlying for strategy"})
		case errors.Is(err, services.ErrNoInstrument):
			c.JSON(http.StatusNotFound, gin.H{"error": "no instrument found for underlying"})
		case errors.Is(err, services.ErrNoCandleData):
			c.JSON(http.StatusBadRequest, gin.H{"error": "no candle data available"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "backtest failed"})
		}
		return
	}

	c.JSON(http.StatusAccepted, toBacktestSubmitResponse(run))
}

// List returns completed backtest runs ordered by most recent completion first.
func (h *BacktestHandler) List(c *gin.Context) {
	page, limit := parseBacktestListPagination(c)
	runs, total, err := h.backtestSvc.ListCompleted(c.Request.Context(), page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list backtests"})
		return
	}
	if runs == nil {
		runs = []models.BacktestRun{}
	}
	resp, err := toBacktestListResponse(runs, page, limit, total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to shape backtests list"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetByID returns the current state of a backtest run.
// PENDING/RUNNING: returns id and status only.
// COMPLETED: returns id, status, and the full result payload.
// FAILED: returns id, status, and errorMessage.
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

	c.JSON(http.StatusOK, toBacktestStatusResponse(run))
}

// GetTrades returns a paginated list of individual trades for a completed backtest.
// Query params: page (default 1), limit (default 50, max 200).
func (h *BacktestHandler) GetTrades(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backtest ID"})
		return
	}

	run, err := h.backtestSvc.GetByIDWithTrades(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "backtest not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get backtest"})
		return
	}

	if run.Status != models.BacktestCompleted {
		c.JSON(http.StatusConflict, gin.H{"error": "backtest not completed"})
		return
	}

	page, limit := parseTradesPagination(c)
	resp, err := toBacktestTradesPageResponse(run.Trades, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseBacktestListPagination(c *gin.Context) (page, limit int) {
	return parsePagination(c, defaultBacktestsPage, defaultBacktestsLimit, maxBacktestsLimit)
}

func parseTradesPagination(c *gin.Context) (page, limit int) {
	return parsePagination(c, defaultTradesPage, defaultTradesLimit, maxTradesLimit)
}

func parsePagination(c *gin.Context, defaultPage, defaultLimit, maxLimit int) (page, limit int) {
	page = defaultPage
	limit = defaultLimit
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		if v > maxLimit {
			v = maxLimit
		}
		limit = v
	}
	return
}
