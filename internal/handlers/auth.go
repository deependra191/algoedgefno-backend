package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
)

// sessionRequest is the JSON body for POST /api/v1/auth/session.
type sessionRequest struct {
	FirebaseIdToken string `json:"firebaseIdToken" binding:"required"`
}

// refreshRequest is the JSON body for POST /api/v1/auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// logoutRequest is the JSON body for POST /api/v1/auth/logout.
type logoutRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// debugSessionRequest is the JSON body for POST /api/v1/auth/debug-session.
type debugSessionRequest struct {
	UID         string `json:"uid"         binding:"required"`
	Email       string `json:"email"       binding:"required"`
	DisplayName string `json:"displayName"`
}

// AuthHandler handles HTTP requests for Firebase-based authentication.
type AuthHandler struct {
	authSvc *services.AuthService
	env     config.Environment
}

// NewAuthHandler constructs an AuthHandler. env is used to gate the
// debug-session endpoint (only in development and test).
func NewAuthHandler(authSvc *services.AuthService, env config.Environment) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, env: env}
}

// Session exchanges a Firebase ID token for a backend access + refresh token
// pair. POST /api/v1/auth/session.
//
// Error mapping:
//   - bind error or oversized token → 400 invalid_request
//   - ErrFirebaseNotConfigured      → 503 firebase_not_configured
//   - ErrFirebaseTokenInvalid       → 401 firebase_token_invalid
//   - ErrFirebaseEmailUnverified    → 403 firebase_email_unverified
//   - ErrAuthNotAllowed             → 403 auth_not_allowed
//   - ErrIdentityConflict           → 409 identity_conflict
//   - ErrInvalidRequest             → 400 invalid_request
//   - any other error               → 500 internal
func (h *AuthHandler) Session(c *gin.Context) {
	var req sessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	// Belt-and-suspenders length check: RequestBodyLimit middleware already
	// caps the body at 8 KiB; this guards against very long tokens that
	// somehow slip through.
	if len(req.FirebaseIdToken) > 4096 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	result, err := h.authSvc.ExchangeFirebaseToken(c.Request.Context(), req.FirebaseIdToken)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, toAuthResponse(result))
	case errors.Is(err, models.ErrFirebaseNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "firebase_not_configured"})
	case errors.Is(err, models.ErrFirebaseTokenInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "firebase_token_invalid"})
	case errors.Is(err, models.ErrFirebaseEmailUnverified):
		c.JSON(http.StatusForbidden, gin.H{"error": "firebase_email_unverified"})
	case errors.Is(err, models.ErrAuthNotAllowed):
		c.JSON(http.StatusForbidden, gin.H{"error": "auth_not_allowed"})
	case errors.Is(err, models.ErrIdentityConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "identity_conflict"})
	case errors.Is(err, models.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	}
}

// Refresh exchanges a refresh token for a new access + refresh token pair.
// POST /api/v1/auth/refresh.
//
// Error mapping:
//   - bind error             → 400 invalid_request
//   - ErrInvalidRequest      → 400 invalid_request
//   - ErrRefreshTokenInvalid → 401 refresh_token_invalid
//   - any other error        → 500 internal  (NOT 401 — see §H comment)
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	pair, err := h.authSvc.RefreshSession(c.Request.Context(), req.RefreshToken)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, refreshResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken})
	case errors.Is(err, models.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
	case errors.Is(err, models.ErrRefreshTokenInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh_token_invalid"})
	default:
		// Wrapped repository / transport failure. Do NOT collapse to 401:
		// that would log the owner out on a transient DB blip AND silence
		// an operational incident behind a routine-looking auth failure.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	}
}

// Logout revokes the supplied refresh token. POST /api/v1/auth/logout.
//
// Idempotent — an absent or already-revoked token returns 204.
// RevokeByHash failure returns 500 (a DB error must not silently succeed).
//
// Error mapping:
//   - bind error        → 400 invalid_request
//   - ErrInvalidRequest → 400 invalid_request
//   - nil               → 204 No Content
//   - any other error   → 500 internal
func (h *AuthHandler) Logout(c *gin.Context) {
	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	err := h.authSvc.Logout(c.Request.Context(), req.RefreshToken)
	switch {
	case err == nil:
		c.Status(http.StatusNoContent)
	case errors.Is(err, models.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	}
}

// DebugSession mints a session for a synthetic identity without requiring a
// real Firebase ID token. Only available in development and test environments.
// POST /api/v1/auth/debug-session.
func (h *AuthHandler) DebugSession(c *gin.Context) {
	var req debugSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	result, err := h.authSvc.DebugSession(c.Request.Context(), req.UID, req.Email, req.DisplayName)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, toAuthResponse(result))
	case errors.Is(err, models.ErrNotAvailable):
		c.JSON(http.StatusNotFound, gin.H{"error": "not_available"})
	case errors.Is(err, models.ErrIdentityConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "identity_conflict"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	}
}
