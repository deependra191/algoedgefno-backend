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
// Returns 503 if any check fails. The error message is generic and does not expose
// DB details, DSNs, or identity values.
func (h *HealthHandler) Ready(c *gin.Context) {
	checks, err := h.svc.CheckReadiness(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, readyResponse{
		Status: "ok",
		Checks: readyChecks{
			Database:         checks.Database,
			DatabaseIdentity: checks.DatabaseIdentity,
		},
	})
}

// Version handles GET /version. It returns build metadata injected at image build time
// and the current migration version (best-effort). MigrationVersion is omitted from the
// response if the DB query fails; the endpoint always returns 200.
func (h *HealthHandler) Version(c *gin.Context) {
	migVersion, err := h.svc.MigrationVersion(c.Request.Context())
	var mv *int64
	if err == nil {
		mv = &migVersion
	}
	c.JSON(http.StatusOK, versionResponse{
		AppVersion:       buildinfo.AppVersion,
		CommitSHA:        buildinfo.CommitSHA,
		BuildTime:        buildinfo.BuildTime,
		Environment:      string(h.env),
		MigrationVersion: mv,
	})
}
