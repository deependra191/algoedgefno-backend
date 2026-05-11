package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRequestIDRouter() *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/", func(c *gin.Context) {
		id, _ := c.Get(middleware.RequestIDKey)
		c.String(http.StatusOK, id.(string))
	})
	return r
}

// TestRequestID_ValidHeader_Preserved verifies that a well-formed UUID in the
// X-Request-ID request header is reused in the context and response header.
func TestRequestID_ValidHeader_Preserved(t *testing.T) {
	want := uuid.New().String()
	r := newRequestIDRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.RequestIDHeader, want)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", w.Code)
	}
	if got := w.Body.String(); got != want {
		t.Errorf("context request_id: got %q want %q", got, want)
	}
	if got := w.Header().Get(middleware.RequestIDHeader); got != want {
		t.Errorf("response header %s: got %q want %q", middleware.RequestIDHeader, got, want)
	}
}

// TestRequestID_InvalidHeader_GeneratesNew verifies that a malformed header
// value is replaced with a freshly generated UUID.
func TestRequestID_InvalidHeader_GeneratesNew(t *testing.T) {
	r := newRequestIDRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.RequestIDHeader, "not-a-uuid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := w.Header().Get(middleware.RequestIDHeader)
	if _, err := uuid.Parse(got); err != nil {
		t.Errorf("%s response header is not a valid UUID: %q", middleware.RequestIDHeader, got)
	}
	if got == "not-a-uuid" {
		t.Error("invalid input was preserved instead of a new ID being generated")
	}
}

// TestRequestID_MissingHeader_GeneratesNew verifies that an absent header
// results in a freshly generated UUID.
func TestRequestID_MissingHeader_GeneratesNew(t *testing.T) {
	r := newRequestIDRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := w.Header().Get(middleware.RequestIDHeader)
	if _, err := uuid.Parse(got); err != nil {
		t.Errorf("%s response header is not a valid UUID: %q", middleware.RequestIDHeader, got)
	}
}

// TestRequestID_ContextAndResponseMatch verifies that the context value and
// the response header always carry the same ID.
func TestRequestID_ContextAndResponseMatch(t *testing.T) {
	var contextID string
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/", func(c *gin.Context) {
		id, _ := c.Get(middleware.RequestIDKey)
		contextID = id.(string)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	headerID := w.Header().Get(middleware.RequestIDHeader)
	if contextID != headerID {
		t.Errorf("context ID %q != response header %q", contextID, headerID)
	}
}
