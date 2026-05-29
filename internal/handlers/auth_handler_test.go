package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/config"
	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- mock authService for handler tests ---

// mockAuthSvc implements the methods on *services.AuthService used by
// AuthHandler. Because AuthHandler holds a *services.AuthService (concrete),
// we can't inject a pure interface. Instead we test via a real AuthService
// backed by mock repos, constructed through services.NewAuthService.

// buildHandlerWithMocks creates a real *AuthService from mock repos plus an
// AuthHandler, wired into a gin router for handler-level HTTP tests.
func buildHandlerWithMocks(
	userRepo models.UserRepository,
	tokenRepo models.RefreshTokenRepository,
	verifier models.FirebaseVerifier,
	allowedUIDs []string,
	env config.Environment,
) (*gin.Engine, *AuthHandler) {
	authSvc := services.NewAuthService(
		userRepo, tokenRepo, verifier,
		"test-jwt-secret-32-bytes-minimum!!", allowedUIDs, env,
	)
	h := NewAuthHandler(authSvc, env)

	r := gin.New()
	r.POST("/api/v1/auth/session", h.Session)
	r.POST("/api/v1/auth/refresh", h.Refresh)
	r.POST("/api/v1/auth/logout", h.Logout)
	r.POST("/api/v1/auth/debug-session", h.DebugSession)
	return r, h
}

// --- mock repos (mirrored from services tests) ---

type handlerMockUserRepo struct {
	upsertResult *models.User
	upsertErr    error
	findResult   *models.User
	findErr      error
}

func (m *handlerMockUserRepo) UpsertByFirebaseUID(_ context.Context, _ *models.User) (*models.User, error) {
	return m.upsertResult, m.upsertErr
}
func (m *handlerMockUserRepo) FindByID(_ context.Context, _ uuid.UUID) (*models.User, error) {
	return m.findResult, m.findErr
}

type handlerMockTokenRepo struct {
	createErr    error
	lookupResult uuid.UUID
	lookupErr    error
	rotateResult uuid.UUID
	rotateErr    error
	revokeErr    error
}

func (m *handlerMockTokenRepo) Create(_ context.Context, _ *models.RefreshToken) error {
	return m.createErr
}
func (m *handlerMockTokenRepo) LookupActiveUserID(_ context.Context, _ string) (uuid.UUID, error) {
	return m.lookupResult, m.lookupErr
}
func (m *handlerMockTokenRepo) RotateRefreshToken(_ context.Context, _ string, _ *models.RefreshToken) (uuid.UUID, error) {
	return m.rotateResult, m.rotateErr
}
func (m *handlerMockTokenRepo) RevokeByHash(_ context.Context, _ string) error {
	return m.revokeErr
}
func (m *handlerMockTokenRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

type handlerMockVerifier struct {
	result *models.FirebaseClaims
	err    error
}

func (m *handlerMockVerifier) VerifyIDToken(_ context.Context, _ string) (*models.FirebaseClaims, error) {
	return m.result, m.err
}

// --- helper ---

func doPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func assertJSON(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantErrorCode string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, wantStatus, w.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body)
	}
	if body["error"] != wantErrorCode {
		t.Errorf("error = %q, want %q", body["error"], wantErrorCode)
	}
}

// --- Session handler tests ---

// TestSession_OversizedFirebaseIdToken asserts that a firebaseIdToken longer
// than 4096 characters is rejected with 400 invalid_request.
func TestSession_OversizedFirebaseIdToken(t *testing.T) {
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, &handlerMockTokenRepo{}, nil, nil, config.EnvDevelopment)
	oversized := strings.Repeat("A", 4097)
	body := `{"firebaseIdToken":"` + oversized + `"}`
	w := doPost(r, "/api/v1/auth/session", body)
	assertJSON(t, w, http.StatusBadRequest, "invalid_request")
}

// TestSession_FirebaseNotConfigured asserts that a nil verifier returns 503.
func TestSession_FirebaseNotConfigured(t *testing.T) {
	// nil verifier
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, &handlerMockTokenRepo{}, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/session", `{"firebaseIdToken":"valid-looking-token"}`)
	assertJSON(t, w, http.StatusServiceUnavailable, "firebase_not_configured")
}

// TestSession_MissingBody asserts 400 when firebaseIdToken is missing.
func TestSession_MissingBody(t *testing.T) {
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, &handlerMockTokenRepo{}, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/session", `{}`)
	assertJSON(t, w, http.StatusBadRequest, "invalid_request")
}

// --- Refresh handler tests ---

// TestRefresh_MalformedToken asserts that a token of wrong length/charset → 400.
func TestRefresh_MalformedToken(t *testing.T) {
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, &handlerMockTokenRepo{}, nil, nil, config.EnvDevelopment)
	// Length != 43
	w := doPost(r, "/api/v1/auth/refresh", `{"refreshToken":"tooshort"}`)
	assertJSON(t, w, http.StatusBadRequest, "invalid_request")
}

// TestRefresh_LookupDBFailure_Gives500 asserts that a non-ErrNotFound lookup
// error propagates as 500 (not 401).
func TestRefresh_LookupDBFailure_Gives500(t *testing.T) {
	raw := makeValidRawToken(t)
	// For this test we actually use a DB-style error (non-ErrNotFound) to get 500.
	tokenRepo := &handlerMockTokenRepo{lookupErr: models.ErrIdentityConflict} // non-sentinel DB error
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, tokenRepo, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/refresh", `{"refreshToken":"`+raw+`"}`)
	assertJSON(t, w, http.StatusInternalServerError, "internal")
}

// TestRefresh_LookupErrNotFound_Gives401 asserts ErrNotFound from lookup → 401.
func TestRefresh_LookupErrNotFound_Gives401(t *testing.T) {
	raw := makeValidRawToken(t)
	tokenRepo := &handlerMockTokenRepo{lookupErr: models.ErrNotFound}
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, tokenRepo, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/refresh", `{"refreshToken":"`+raw+`"}`)
	assertJSON(t, w, http.StatusUnauthorized, "refresh_token_invalid")
}

// TestRefreshResponseShape asserts that the refresh response has only
// {accessToken, refreshToken} — no user field.
func TestRefreshResponseShape(t *testing.T) {
	raw := makeValidRawToken(t)
	uid := uuid.New()
	u := &models.User{ID: uid, FirebaseUID: "uid-abc"}
	tokenRepo := &handlerMockTokenRepo{lookupResult: uid, rotateResult: uid}
	userRepo := &handlerMockUserRepo{findResult: u}

	r, _ := buildHandlerWithMocks(userRepo, tokenRepo, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/refresh", `{"refreshToken":"`+raw+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["accessToken"]; !ok {
		t.Error("refreshResponse missing accessToken")
	}
	if _, ok := body["refreshToken"]; !ok {
		t.Error("refreshResponse missing refreshToken")
	}
	if _, ok := body["user"]; ok {
		t.Error("refreshResponse must NOT include user field")
	}
}

// --- Logout handler tests ---

// TestLogout_DBFailure_Returns500 asserts that a RevokeByHash error → 500.
func TestLogout_DBFailure_Returns500(t *testing.T) {
	raw := makeValidRawToken(t)
	// Use ErrIdentityConflict as a stand-in for any unexpected DB error.
	tokenRepo := &handlerMockTokenRepo{revokeErr: models.ErrIdentityConflict}
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, tokenRepo, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/logout", `{"refreshToken":"`+raw+`"}`)
	assertJSON(t, w, http.StatusInternalServerError, "internal")
}

// TestLogout_TokenAbsent_Returns204 asserts that revoking an absent token → 204.
// RevokeByHash is idempotent; nil return means 0 rows affected is not an error.
func TestLogout_TokenAbsent_Returns204(t *testing.T) {
	raw := makeValidRawToken(t)
	tokenRepo := &handlerMockTokenRepo{revokeErr: nil} // nil = success (idempotent)
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, tokenRepo, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/logout", `{"refreshToken":"`+raw+`"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body)
	}
}

// TestLogout_MissingBody asserts 400 when refreshToken field is missing.
func TestLogout_MissingBody(t *testing.T) {
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, &handlerMockTokenRepo{}, nil, nil, config.EnvDevelopment)
	w := doPost(r, "/api/v1/auth/logout", `{}`)
	assertJSON(t, w, http.StatusBadRequest, "invalid_request")
}

// --- helper to produce a valid-format raw token via newRefreshToken ---

// newRawToken generates a fresh 43-char base64url refresh token and its
// SHA-256 hex digest, matching the invariant used by the service layer.
func newRawToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf) // 43 chars
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

func makeValidRawToken(t *testing.T) string {
	t.Helper()
	raw, _, err := newRawToken()
	if err != nil {
		t.Fatalf("newRawToken: %v", err)
	}
	return raw
}

// oversizedSessionBody builds a JSON body with a firebaseIdToken of given length.
func oversizedSessionBody(n int) []byte {
	tok := strings.Repeat("A", n)
	b, _ := json.Marshal(map[string]string{"firebaseIdToken": tok})
	return b
}

// TestSession_OversizedBodyViaBytes is a supplemental check using raw byte
// slices to ensure 4097-char tokens are rejected.
func TestSession_OversizedBodyViaBytes(t *testing.T) {
	r, _ := buildHandlerWithMocks(&handlerMockUserRepo{}, &handlerMockTokenRepo{}, nil, nil, config.EnvDevelopment)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", bytes.NewReader(oversizedSessionBody(4097)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assertJSON(t, w, http.StatusBadRequest, "invalid_request")
}
