package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// newRateLimitRouter builds a minimal gin engine using the injectable
// rateLimitWithLimiter so tests control the limiter directly without touching
// the package-level singleton or spawning background goroutines.
func newRateLimitRouter(route string, rpm int, l *ipRateLimiter) *gin.Engine {
	r := gin.New()
	r.POST(route, rateLimitWithLimiter(route, rpm, l), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

// fireRequests sends n POST requests to path on r, returning the slice of HTTP
// status codes received.
func fireRequests(r *gin.Engine, path string, n int) []int {
	codes := make([]int, 0, n)
	for i := 0; i < n; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		r.ServeHTTP(w, req)
		codes = append(codes, w.Code)
	}
	return codes
}

// countStatus tallies how many codes in the slice equal target.
func countStatus(codes []int, target int) int {
	n := 0
	for _, c := range codes {
		if c == target {
			n++
		}
	}
	return n
}

// TestRateLimit_RefreshBurst asserts that bursting 70 requests against a 60
// rpm limit produces at least one 429 rate_limit_exceeded response.
func TestRateLimit_RefreshBurst(t *testing.T) {
	const route = "/api/v1/auth/refresh"
	const rpm = 60
	const burst = 70

	l := newIPRateLimiterWithClock(time.Now)
	r := newRateLimitRouter(route, rpm, l)

	codes := fireRequests(r, route, burst)

	got429 := countStatus(codes, http.StatusTooManyRequests)
	if got429 == 0 {
		t.Fatalf("expected ≥1 429 from %d burst requests at %d rpm, got none", burst, rpm)
	}
	t.Logf("burst=%d rpm=%d → %d requests allowed, %d rejected (429)", burst, rpm, countStatus(codes, http.StatusOK), got429)
}

// TestRateLimit_LogoutBurst asserts that bursting 40 requests against a 30 rpm
// limit produces at least one 429 rate_limit_exceeded response.
func TestRateLimit_LogoutBurst(t *testing.T) {
	const route = "/api/v1/auth/logout"
	const rpm = 30
	const burst = 40

	l := newIPRateLimiterWithClock(time.Now)
	r := newRateLimitRouter(route, rpm, l)

	codes := fireRequests(r, route, burst)

	got429 := countStatus(codes, http.StatusTooManyRequests)
	if got429 == 0 {
		t.Fatalf("expected ≥1 429 from %d burst requests at %d rpm, got none", burst, rpm)
	}
	t.Logf("burst=%d rpm=%d → %d requests allowed, %d rejected (429)", burst, rpm, countStatus(codes, http.StatusOK), got429)
}

// TestRateLimit_429BodyShape asserts that the 429 response body is the
// canonical {"error":"rate_limit_exceeded"} JSON required by CLAUDE.md rule 3.
func TestRateLimit_429BodyShape(t *testing.T) {
	const route = "/api/v1/auth/refresh"
	const rpm = 1

	l := newIPRateLimiterWithClock(time.Now)
	r := newRateLimitRouter(route, rpm, l)

	// First request consumes the 1-rpm budget.
	fireRequests(r, route, 1)

	// Second request must be rejected.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, route, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal 429 body: %v", err)
	}
	if body["error"] != errRateLimitExceeded {
		t.Errorf("expected error %q, got %q", errRateLimitExceeded, body["error"])
	}
}

// TestRateLimit_WindowRollover asserts that the request counter resets after
// rateLimitWindow elapses, allowing a new burst.
func TestRateLimit_WindowRollover(t *testing.T) {
	const route = "/api/v1/auth/refresh"
	const rpm = 5

	// Start the clock at an arbitrary fixed point.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	l := newIPRateLimiterWithClock(func() time.Time { return now })
	r := newRateLimitRouter(route, rpm, l)

	// Exhaust the window.
	codes := fireRequests(r, route, rpm)
	if got := countStatus(codes, http.StatusOK); got != rpm {
		t.Fatalf("expected %d OKs in first window, got %d", rpm, got)
	}

	// Still in the same window — next request must be rejected.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, route, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 while still in window, got %d", w.Code)
	}

	// Advance past the window boundary.
	now = base.Add(rateLimitWindow + time.Millisecond)

	// A fresh burst must succeed again.
	codes2 := fireRequests(r, route, rpm)
	if got := countStatus(codes2, http.StatusOK); got != rpm {
		t.Fatalf("expected %d OKs in new window, got %d", rpm, got)
	}
}

// TestRateLimit_EvictsStaleEntries asserts that evictStale removes entries
// whose lastSeen is older than the supplied TTL, bounding the map size. This is
// the "Rate limiter evicts stale entries / map bounded" deliverable from §14.
func TestRateLimit_EvictsStaleEntries(t *testing.T) {
	const route = "/api/v1/auth/refresh"
	const rpm = 10

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	l := newIPRateLimiterWithClock(func() time.Time { return now })

	// Fire one request to populate the entry map.
	r := newRateLimitRouter(route, rpm, l)
	fireRequests(r, route, 1)

	l.mu.Lock()
	entriesBefore := len(l.entries)
	l.mu.Unlock()

	if entriesBefore == 0 {
		t.Fatal("expected ≥1 entry after request, got 0")
	}

	// Advance time past the idle TTL — the entry should now be stale.
	now = base.Add(rateLimitIdleTTL + time.Second)

	// Drive eviction directly (no goroutine, no sleep).
	l.evictStale(rateLimitIdleTTL)

	l.mu.Lock()
	entriesAfter := len(l.entries)
	l.mu.Unlock()

	if entriesAfter != 0 {
		t.Errorf("expected 0 entries after eviction of stale entry, got %d", entriesAfter)
	}
}

// TestRateLimit_FreshEntryNotEvicted asserts that an entry whose lastSeen is
// within the TTL is preserved by evictStale.
func TestRateLimit_FreshEntryNotEvicted(t *testing.T) {
	const route = "/api/v1/auth/logout"
	const rpm = 10

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	l := newIPRateLimiterWithClock(func() time.Time { return now })

	r := newRateLimitRouter(route, rpm, l)
	fireRequests(r, route, 1)

	// Advance to just before the TTL boundary.
	now = base.Add(rateLimitIdleTTL - time.Second)
	l.evictStale(rateLimitIdleTTL)

	l.mu.Lock()
	entriesAfter := len(l.entries)
	l.mu.Unlock()

	if entriesAfter == 0 {
		t.Error("expected fresh entry to survive eviction, but map is empty")
	}
}
