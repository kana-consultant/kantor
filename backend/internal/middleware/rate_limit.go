package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

type ipRateLimiter struct {
	maxRequests int
	window      time.Duration

	mu      sync.Mutex
	entries map[string]rateLimitEntry
}

type rateLimitEntry struct {
	count   int
	resetAt time.Time
}

func NewIPRateLimit(maxRequests int, window time.Duration, code string, message string) func(http.Handler) http.Handler {
	limiter := &ipRateLimiter{
		maxRequests: maxRequests,
		window:      window,
		entries:     make(map[string]rateLimitEntry),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			retryAfter := limiter.retryAfter(clientIPFromRequest(r), time.Now().UTC())
			if retryAfter > 0 {
				retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
				if retryAfterSeconds < 1 {
					retryAfterSeconds = 1
				}

				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
				response.WriteError(w, http.StatusTooManyRequests, code, message, map[string]int{
					"retry_after_seconds": retryAfterSeconds,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (l *ipRateLimiter) retryAfter(key string, now time.Time) time.Duration {
	if key == "" {
		key = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.entries) > 1024 {
		for candidate, entry := range l.entries {
			if now.After(entry.resetAt) {
				delete(l.entries, candidate)
			}
		}
		// Hard cap: if most entries are still live, nuke everything
		// to prevent OOM under distributed attack
		if len(l.entries) > 10000 {
			clear(l.entries)
		}
	}

	entry, exists := l.entries[key]
	if !exists || now.After(entry.resetAt) {
		l.entries[key] = rateLimitEntry{
			count:   1,
			resetAt: now.Add(l.window),
		}
		return 0
	}

	if entry.count >= l.maxRequests {
		return entry.resetAt.Sub(now)
	}

	entry.count++
	l.entries[key] = entry
	return 0
}

// clientIPFromRequest returns the client IP from r.RemoteAddr.
// Assumes chi's RealIP middleware has already rewritten RemoteAddr
// from X-Real-IP / X-Forwarded-For when behind a reverse proxy.
func clientIPFromRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}
