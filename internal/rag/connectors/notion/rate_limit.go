package notion

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// notionMaxAttempts bounds retries per request. Notion recommends a
// small number of retries with backoff; three matches the reference
// behaviour.
const notionMaxAttempts = 3

// The following are vars (not consts) so tests can shrink the waits into
// the millisecond range.
var (
	notionRetryBackoff    = 500 * time.Millisecond
	notionRateLimitMargin = 1 * time.Second
)

// rateLimitError signals a 429 response; retryAt is when the connector
// may try again (Retry-After seconds from now).
type rateLimitError struct {
	retryAt time.Time
}

func (e *rateLimitError) Error() string {
	return "notion: rate limited until " + e.retryAt.UTC().Format(time.RFC3339)
}

// detectRateLimit reports whether a response is a 429 and, if so, when
// the request may be retried. A missing or unparsable Retry-After
// header defaults to a one-second wait.
func detectRateLimit(status int, headers http.Header) (time.Time, bool) {
	if status != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	secs, err := strconv.Atoi(strings.TrimSpace(headers.Get("Retry-After")))
	if err != nil || secs < 0 {
		secs = 1
	}
	return time.Now().Add(time.Duration(secs) * time.Second), true
}

// withRetry runs op up to notionMaxAttempts times. A rateLimitError
// pauses until its retryAt (plus a small margin); any other error backs
// off exponentially before the next attempt. Context cancellation
// aborts the wait.
func withRetry(ctx context.Context, op func() error) error {
	var lastErr error
	backoff := notionRetryBackoff

	for attempt := 0; attempt < notionMaxAttempts; attempt++ {
		err := op()
		if err == nil {
			return nil
		}
		lastErr = err

		var rl *rateLimitError
		if errors.As(err, &rl) {
			wait := time.Until(rl.retryAt.Add(notionRateLimitMargin))
			if wait > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
			continue
		}

		if attempt < notionMaxAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
	}
	return lastErr
}
