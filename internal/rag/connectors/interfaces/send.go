package interfaces

import "context"

// Send delivers v on out, returning false if ctx is cancelled first. Producers
// must use this for every send: consumers routinely abandon connector streams
// early, and a bare blocking send on the unread channel leaks the producer.
func Send[T any](ctx context.Context, out chan<- T, v T) bool {
	select {
	case out <- v:
		return true
	case <-ctx.Done():
		return false
	}
}
