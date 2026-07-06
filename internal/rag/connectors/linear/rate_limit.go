package linear

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Attempt bounds. RATELIMITED responses get more attempts because they
// resolve deterministically once the reset window passes; transient
// 5xx/network errors get a small exponential-backoff budget.
const (
	maxTransientAttempts = 3
	maxRateLimitAttempts = 5
)

// Timings are vars (not consts) so tests can shrink them.
var (
	retryBackoff         = 500 * time.Millisecond
	defaultRateLimitWait = 30 * time.Second
	maxRateLimitWait     = 60 * time.Second
)

// detectRateLimit reports whether a response is a throttle and, if so,
// how long to wait before retrying. Linear signals throttling with an
// HTTP 400 carrying a GraphQL error whose extensions.code is
// "RATELIMITED"; we also treat a defensive 429 as a throttle.
func detectRateLimit(status int, headers http.Header, body []byte, now time.Time) (time.Duration, bool) {
	throttled := status == http.StatusTooManyRequests
	if status == http.StatusBadRequest && isRateLimitedBody(body) {
		throttled = true
	}
	if !throttled {
		return 0, false
	}
	return rateLimitWait(headers, now), true
}

// isRateLimitedBody reports whether a GraphQL error body carries a
// RATELIMITED extensions code.
func isRateLimitedBody(body []byte) bool {
	var resp graphQLResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	for _, e := range resp.Errors {
		if strings.EqualFold(e.Extensions.Code, "RATELIMITED") {
			return true
		}
	}
	return false
}

// rateLimitWait derives the retry delay from the
// X-RateLimit-Requests-Reset header. Linear documents the unit
// inconsistently, so the value is disambiguated by magnitude: a value
// large enough to be a millisecond epoch (> 1e12, i.e. year 2001+ in ms)
// is treated as milliseconds, otherwise as seconds. The result is
// clamped to [0, maxRateLimitWait]; a missing/unparsable header falls
// back to defaultRateLimitWait.
func rateLimitWait(headers http.Header, now time.Time) time.Duration {
	raw := strings.TrimSpace(headers.Get("X-RateLimit-Requests-Reset"))
	if raw == "" {
		return defaultRateLimitWait
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return defaultRateLimitWait
	}

	var reset time.Time
	if n > 1e12 {
		reset = time.UnixMilli(n)
	} else {
		reset = time.Unix(n, 0)
	}

	wait := reset.Sub(now)
	if wait <= 0 {
		return 0
	}
	if wait > maxRateLimitWait {
		return maxRateLimitWait
	}
	return wait
}
