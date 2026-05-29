package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Rate-limit tunables. All values with branching logic are named constants so
// the compiler catches typos and readers understand intent without comments.
const (
	// rateLimitWindow is the rolling window over which rpm counts are measured.
	rateLimitWindow = time.Minute

	// rateLimitIdleTTL is the duration of inactivity after which a per-IP entry
	// is considered stale and eligible for eviction.
	rateLimitIdleTTL = 10 * time.Minute

	// rateLimitEvictInterval is how often the background goroutine sweeps for
	// stale entries.
	rateLimitEvictInterval = 5 * time.Minute

	// errRateLimitExceeded is the error code returned in the JSON body on 429.
	errRateLimitExceeded = "rate_limit_exceeded"
)

// rateLimitKey uniquely identifies a per-(route, ip) counter bucket.
type rateLimitKey struct {
	route string
	ip    string
}

// rateLimitEntry tracks request counts within the current window for one
// (route, ip) pair.
type rateLimitEntry struct {
	// windowStart is the beginning of the current fixed window.
	windowStart time.Time
	// count is the number of requests seen in the current window.
	count int
	// lastSeen is updated on every request; the eviction sweep uses it to
	// detect idle entries.
	lastSeen time.Time
}

// ipRateLimiter is a shared, mutex-protected store of per-(route, ip) counters.
// A background goroutine periodically evicts entries that have been idle for
// rateLimitIdleTTL, bounding the memory footprint under sustained multi-IP
// traffic.
type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[rateLimitKey]*rateLimitEntry
	// now is a replaceable clock used by tests for deterministic time control.
	now func() time.Time
}

// newIPRateLimiter constructs a limiter and starts the background eviction
// goroutine. Callers that need deterministic eviction (e.g. tests) should use
// newIPRateLimiterWithClock instead and call evictStale directly.
func newIPRateLimiter() *ipRateLimiter {
	return newIPRateLimiterWithClock(time.Now)
}

// newIPRateLimiterWithClock constructs a limiter with an injectable clock. The
// background eviction goroutine is NOT started; callers drive eviction manually
// via evictStale. This is the preferred constructor for tests.
func newIPRateLimiterWithClock(now func() time.Time) *ipRateLimiter {
	return &ipRateLimiter{
		entries: make(map[rateLimitKey]*rateLimitEntry),
		now:     now,
	}
}

// startEviction launches the background goroutine that sweeps for stale entries
// every rateLimitEvictInterval. It is called once by the package-level limiter
// and never by test constructors.
func (l *ipRateLimiter) startEviction() {
	go func() {
		ticker := time.NewTicker(rateLimitEvictInterval)
		defer ticker.Stop()
		for range ticker.C {
			l.evictStale(rateLimitIdleTTL)
		}
	}()
}

// evictStale removes entries whose lastSeen is older than ttl relative to the
// limiter's clock. It is unexported-but-accessible from tests in the same
// package, allowing deterministic eviction assertions without spawning
// goroutines or sleeping.
func (l *ipRateLimiter) evictStale(ttl time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, e := range l.entries {
		if now.Sub(e.lastSeen) > ttl {
			delete(l.entries, k)
		}
	}
}

// allow returns true if the (route, ip) pair is within the rpm budget for the
// current window, incrementing the counter. It returns false when the budget is
// exhausted, without incrementing (the rejected request does not consume quota).
func (l *ipRateLimiter) allow(route, ip string, rpm int) bool {
	key := rateLimitKey{route: route, ip: ip}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok {
		l.entries[key] = &rateLimitEntry{
			windowStart: now,
			count:       1,
			lastSeen:    now,
		}
		return true
	}

	// Roll over to a new window when the previous one has expired.
	if now.Sub(e.windowStart) >= rateLimitWindow {
		e.windowStart = now
		e.count = 0
	}

	if e.count >= rpm {
		// Do not update lastSeen on rejected requests so that an attacker
		// flooding a dead-IP bucket cannot keep it alive indefinitely.
		return false
	}

	e.count++
	e.lastSeen = now
	return true
}

// packageLimiter is the singleton shared across all RateLimit middleware
// instances for the lifetime of the process. One limiter handles all routes;
// the key space is already segmented by (route, ip).
var packageLimiter = func() *ipRateLimiter {
	l := newIPRateLimiter()
	l.startEviction()
	return l
}()

// RateLimit returns a Gin middleware that enforces a per-IP fixed-window rate
// limit of rpm requests per minute for the named route. Excess requests receive
// 429 with body {"error":"rate_limit_exceeded"}.
//
// route should be the canonical route path string (e.g. "/api/v1/auth/refresh")
// so that different routes do not share the same counter bucket.
//
// The middleware is safe for concurrent use; all state is protected by an
// internal mutex.
func RateLimit(route string, rpm int) gin.HandlerFunc {
	return rateLimitWithLimiter(route, rpm, packageLimiter)
}

// rateLimitWithLimiter is the injectable variant used by tests to supply a
// controlled limiter without touching the package-level singleton.
func rateLimitWithLimiter(route string, rpm int, l *ipRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !l.allow(route, ip, rpm) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": errRateLimitExceeded})
			return
		}
		c.Next()
	}
}
