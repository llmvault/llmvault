package linear

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// rateLimitError signals a throttle; wait is how long to pause before the
// next attempt.
type rateLimitError struct{ wait time.Duration }

func (e *rateLimitError) Error() string {
	return fmt.Sprintf("linear: rate limited, retry after %s", e.wait)
}

// permanentError wraps an error that must not be retried (GraphQL-level
// errors, non-throttle 4xx, decode failures).
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// joinErrors renders a GraphQL errors array into a single message.
func joinErrors(errs []graphQLError) string {
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Message)
	}
	return strings.Join(msgs, "; ")
}

// defaultSleep is the production sleeper: it waits for d or aborts on
// context cancellation. Tests swap Client.sleep for a recorder.
func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// withRetry runs op with two independent budgets. A rateLimitError pauses
// for its wait and retries up to maxRateLimitAttempts; any other
// (transient) error backs off exponentially and retries up to
// maxTransientAttempts; a permanentError returns immediately. The sleep
// respects context cancellation via Client.sleep.
func (c *Client) withRetry(ctx context.Context, op func() error) error {
	rlAttempts := 0
	transientAttempts := 0
	backoff := retryBackoff

	for {
		err := op()
		if err == nil {
			return nil
		}

		var perm *permanentError
		if errors.As(err, &perm) {
			return err
		}

		var rl *rateLimitError
		if errors.As(err, &rl) {
			rlAttempts++
			if rlAttempts >= maxRateLimitAttempts {
				return err
			}
			if e := c.sleep(ctx, rl.wait); e != nil {
				return e
			}
			continue
		}

		transientAttempts++
		if transientAttempts >= maxTransientAttempts {
			return err
		}
		if e := c.sleep(ctx, backoff); e != nil {
			return e
		}
		backoff *= 2
	}
}
