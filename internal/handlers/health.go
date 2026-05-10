package handlers

import (
	"net/http"

	"github.com/deependra191/algoedgefno-backend/internal/buildinfo"
	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/services"
	"github.com/gin-gonic/gin"
)

// HealthHandler serves liveness, readiness, and version endpoints.
type HealthHandler struct {
	svc *services.HealthService
	env config.Environment
}

// NewHealthHandler creates a HealthHandler for the given environment.
func NewHealthHandler(svc *services.HealthService, env config.Environment) *HealthHandler {
	return &HealthHandler{svc: svc, env: env}
}

// Live handles GET /health. It confirms the process is running with no external checks.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready handles GET /ready. It verifies database connectivity and, for production
// and staging environments, confirms the database identity matches APP_ENV.
// Returns 503 if any check fails.
func (h *HealthHandler) Ready(c *gin.Context) {
	if err := h.svc.CheckReadiness(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, readyResponse{Status: "ok", DB: "connected"})
}

// Version handles GET /version. It returns build metadata and the current migration version.
// The migration version query is best-effort; a query failure returns 0 without failing the endpoint.
func (h *HealthHandler) Version(c *gin.Context) {
	migVersion, _ := h.svc.MigrationVersion(c.Request.Context())
	c.JSON(http.StatusOK, versionResponse{
		AppVersion:       buildinfo.AppVersion,
		CommitSHA:        buildinfo.CommitSHA,
		BuildTime:        buildinfo.BuildTime,
		Environment:      string(h.env),
		MigrationVersion: migVersion,
	})
}
