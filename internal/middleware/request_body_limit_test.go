package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newBodyLimitRouter builds a minimal gin engine with RequestBodyLimit(maxBytes)
// applied. The single POST handler attempts ShouldBindJSON into a generic map
// and returns 400 {"error":"invalid_request"} on any bind error, mirroring the
// Phase-4 handler behaviour so that oversized-body cap triggers are observable.
func newBodyLimitRouter(maxBytes int64) *gin.Engine {
	r := gin.New()
	r.POST("/test", RequestBodyLimit(maxBytes), func(c *gin.Context) {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		c.Status(http.StatusOK)
	})
	return r
}

// postBody fires a POST request with body b against path on router r.
func postBody(r *gin.Engine, path string, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// TestRequestBodyLimit_OversizedSessionBody asserts that a body exceeding the
// 8 KiB session cap causes ShouldBindJSON to fail, returning 400 invalid_request.
// Body is 8*1024+1 bytes — one byte past the cap.
func TestRequestBodyLimit_OversizedSessionBody(t *testing.T) {
	const sessionCap int64 = 8 << 10 // 8 KiB
	r := newBodyLimitRouter(sessionCap)

	// Build a JSON object whose value string pushes total body past the cap.
	// The surrounding JSON punctuation is ~22 bytes; pad value to exceed the cap.
	padLen := int(sessionCap) + 1
	body := make([]byte, padLen)
	for i := range body {
		body[i] = 'A'
	}
	// Wrap in a JSON string field so it is syntactically valid up to the cut-off.
	payload := append([]byte(`{"firebaseIdToken":"`), body...)
	payload = append(payload, []byte(`"}`)...)

	w := postBody(r, "/test", payload)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized session body, got %d; body: %s", w.Code, w.Body)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "invalid_request" {
		t.Errorf("expected error %q, got %q", "invalid_request", resp["error"])
	}
}

// TestRequestBodyLimit_OversizedRefreshBody asserts that a 513-byte body against
// a 512-byte cap (refresh/logout shape) is rejected with 400 invalid_request.
func TestRequestBodyLimit_OversizedRefreshBody(t *testing.T) {
	const refreshCap int64 = 512
	r := newBodyLimitRouter(refreshCap)

	// 513-byte body, one past the cap.
	body := bytes.Repeat([]byte("x"), 513)

	w := postBody(r, "/test", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 513-byte body against 512 B cap, got %d; body: %s", w.Code, w.Body)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "invalid_request" {
		t.Errorf("expected error %q, got %q", "invalid_request", resp["error"])
	}
}

// TestRequestBodyLimit_WithinCapPasses asserts that a body within the cap is
// passed through to the handler without error.
func TestRequestBodyLimit_WithinCapPasses(t *testing.T) {
	const cap int64 = 512
	r := newBodyLimitRouter(cap)

	payload := []byte(`{"refreshToken":"abc"}`)
	w := postBody(r, "/test", payload)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for body within cap, got %d; body: %s", w.Code, w.Body)
	}
}
