package interfaces

import "context"

// Send delivers v on out, aborting if ctx is cancelled first. It returns
// false when the context was cancelled (the caller should stop producing
// and let its goroutine unwind), true on a successful send.
//
// Producer goroutines must use this for every channel send: consumers
// routinely abandon connector streams early (prune diffs, ingest errors,
// perm-sync failures), and a bare blocking send on the unread channel
// leaks the producer goroutine for the lifetime of the process.
func Send[T any](ctx context.Context, out chan<- T, v T) bool {
	select {
	case out <- v:
		return true
	case <-ctx.Done():
		return false
	}
}
