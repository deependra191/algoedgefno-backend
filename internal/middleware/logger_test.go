package middleware_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/middleware"
)

// recordingHandler captures slog records for test assertions.
// Base attributes added via logger.With(...) are not stored — only per-record
// attributes logged by the middleware itself are captured.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

// attrByKey returns the last-recorded attribute matching key, if any.
func (h *recordingHandler) attrByKey(key string) (slog.Attr, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return slog.Attr{}, false
	}
	r := h.records[len(h.records)-1]
	var found slog.Attr
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func newLoggerRouter(l *slog.Logger) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(l))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

// TestLogger_LogsRequestFields asserts that all expected structured fields are
// present in the log record emitted for a completed request.
func TestLogger_LogsRequestFields(t *testing.T) {
	h := &recordingHandler{}
	l := slog.New(h)
	r := newLoggerRouter(l)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	for _, key := range []string{"request_id", "method", "path", "status", "latency_ms"} {
		if _, ok := h.attrByKey(key); !ok {
			t.Errorf("expected log attr %q not found", key)
		}
	}
}

// TestLogger_RequestIDFromContext verifies that the logged request_id matches
// the UUID propagated by the RequestID middleware.
func TestLogger_RequestIDFromContext(t *testing.T) {
	h := &recordingHandler{}
	l := slog.New(h)
	r := newLoggerRouter(l)

	want := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.RequestIDHeader, want)
	r.ServeHTTP(httptest.NewRecorder(), req)

	attr, ok := h.attrByKey(middleware.LogAttrRequestID)
	if !ok {
		t.Fatal("request_id not logged")
	}
	if got := attr.Value.String(); got != want {
		t.Errorf("logged request_id: got %q want %q", got, want)
	}
}

// TestLogger_AuthHeaderNotLogged verifies that the Authorization header value
// never appears in any logged attribute key or value.
func TestLogger_AuthHeaderNotLogged(t *testing.T) {
	h := &recordingHandler{}
	l := slog.New(h)
	r := newLoggerRouter(l)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	r.ServeHTTP(httptest.NewRecorder(), req)

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		t.Fatal("no log records captured")
	}
	rec := h.records[len(h.records)-1]
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "authorization" || a.Key == "Authorization" {
			t.Errorf("Authorization header key was logged: %q", a.Key)
		}
		if a.Value.String() == "Bearer super-secret-token" {
			t.Errorf("token value was logged under attr key %q", a.Key)
		}
		return true
	})
}

// TestLogger_StatusLogged verifies that the HTTP status code returned by the
// handler is what appears in the log record.
func TestLogger_StatusLogged(t *testing.T) {
	h := &recordingHandler{}
	l := slog.New(h)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(l))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNotFound) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	attr, ok := h.attrByKey("status")
	if !ok {
		t.Fatal("status not logged")
	}
	if got := int(attr.Value.Int64()); got != http.StatusNotFound {
		t.Errorf("logged status: got %d want %d", got, http.StatusNotFound)
	}
}

// TestLogger_PanicLogsStatus500AndRequestID verifies that when a handler panics,
// Logger still emits exactly one record with status=500 and the correct request_id.
// This requires the middleware chain order RequestID → Logger → Recovery → handler:
// Recovery catches the panic and returns normally, so Logger's post-c.Next() code runs.
func TestLogger_PanicLogsStatus500AndRequestID(t *testing.T) {
	h := &recordingHandler{}
	l := slog.New(h)
	want := uuid.New().String()

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger(l))
	r.Use(gin.RecoveryWithWriter(io.Discard)) // discard gin's panic output in test
	r.GET("/panic", func(_ *gin.Context) { panic("boom") })

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(middleware.RequestIDHeader, want)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 from panicking handler, got %d", w.Code)
	}

	h.mu.Lock()
	n := len(h.records)
	h.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected exactly 1 log record, got %d", n)
	}

	statusAttr, ok := h.attrByKey("status")
	if !ok {
		t.Fatal("status not logged")
	}
	if got := int(statusAttr.Value.Int64()); got != http.StatusInternalServerError {
		t.Errorf("logged status: got %d want 500", got)
	}

	ridAttr, ok := h.attrByKey(middleware.LogAttrRequestID)
	if !ok {
		t.Fatal("request_id not logged")
	}
	if got := ridAttr.Value.String(); got != want {
		t.Errorf("logged request_id: got %q want %q", got, want)
	}
}

// TestLogger_QueryStringNotLogged verifies that query parameters are never included
// in the logged path. The logger uses URL.Path (not RequestURI) precisely to avoid
// logging tokens or other sensitive data passed as query strings.
func TestLogger_QueryStringNotLogged(t *testing.T) {
	h := &recordingHandler{}
	l := slog.New(h)
	r := newLoggerRouter(l)

	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	pathAttr, ok := h.attrByKey("path")
	if !ok {
		t.Fatal("path not logged")
	}
	if got := pathAttr.Value.String(); got != "/" {
		t.Errorf("logged path: got %q want \"/\" — query string must not be included", got)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		t.Fatal("no log records captured")
	}
	h.records[len(h.records)-1].Attrs(func(a slog.Attr) bool {
		v := a.Value.String()
		if strings.Contains(v, "secret") {
			t.Errorf("query param value %q leaked in attr %q", v, a.Key)
		}
		return true
	})
}
