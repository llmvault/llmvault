package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiterIdleTTL is how long an org's limiter survives without traffic
// before the sweep evicts it. Bounds the map so a long-running process that
// has seen many orgs does not leak a limiter per org forever.
const rateLimiterIdleTTL = 15 * time.Minute

// rateLimiterSweepInterval bounds how often the eviction sweep runs (amortised
// onto request handling; no background goroutine).
const rateLimiterSweepInterval = time.Minute

type orgLimiter struct {
	limiter *rate.Limiter
	// rpm is the RateLimit the limiter was last configured for, used to detect configuration changes.
	rpm      int
	lastSeen time.Time
}

// RateLimit enforces per-org rate limiting from the org's RateLimit field (rpm,
// set on the context by OrgAuth), returning 429 with Retry-After when exceeded.
// Limiters are cached per org; a limit change is applied in place via
// SetLimit/SetBurst (keeping the in-flight bucket).
func RateLimit() func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*orgLimiter)
	lastSweep := time.Now()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			org, ok := OrgFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "org not found in context"})
				return
			}

			orgID := org.ID.String()
			// rate.Limit is events per second; org.RateLimit is per minute.
			rps := rate.Limit(float64(org.RateLimit) / 60.0)
			burst := max(org.RateLimit/10, 1)
			now := time.Now()

			mu.Lock()
			if now.Sub(lastSweep) >= rateLimiterSweepInterval {
				for id, ol := range limiters {
					if now.Sub(ol.lastSeen) > rateLimiterIdleTTL {
						delete(limiters, id)
					}
				}
				lastSweep = now
			}
			ol, exists := limiters[orgID]
			if !exists {
				ol = &orgLimiter{limiter: rate.NewLimiter(rps, burst), rpm: org.RateLimit}
				limiters[orgID] = ol
			} else if ol.rpm != org.RateLimit {
				// Plan changed: re-point the existing bucket at the new ceiling.
				ol.limiter.SetLimit(rps)
				ol.limiter.SetBurst(burst)
				ol.rpm = org.RateLimit
			}
			ol.lastSeen = now
			limiter := ol.limiter
			mu.Unlock()

			if !limiter.Allow() {
				retryAfter := time.Second
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
