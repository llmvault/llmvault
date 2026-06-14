package tasks

import (
	"sync"

	"github.com/hibiken/asynq"
)

// TaskBuilder rebuilds a task and its enqueue options from a stored payload.
// Options are returned separately so callers can re-apply the original
// Queue/MaxRetry/Timeout when re-enqueuing.
type TaskBuilder func(payload []byte) (*asynq.Task, []asynq.Option, error)

var (
	taskBuildersMu sync.RWMutex
	taskBuilders   = map[string]TaskBuilder{}
)

// RegisterTaskBuilder registers a builder for an event type. Task definitions
// call this from init() so a task can be rebuilt from a stored payload.
func RegisterTaskBuilder(eventType string, fn TaskBuilder) {
	taskBuildersMu.Lock()
	defer taskBuildersMu.Unlock()
	taskBuilders[eventType] = fn
}

// lookupTaskBuilder returns the builder registered for an event type, if any.
func lookupTaskBuilder(eventType string) (TaskBuilder, bool) {
	taskBuildersMu.RLock()
	defer taskBuildersMu.RUnlock()
	fn, ok := taskBuilders[eventType]
	return fn, ok
}
