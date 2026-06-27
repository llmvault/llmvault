package logging

import (
	"context"
	"time"
)

// LogPhase emits a structured timing record for one step in a larger operation.
func LogPhase(ctx context.Context, event string, phase string, started time.Time, attrs ...any) {
	fields := make([]any, 0, len(attrs)+6)
	fields = append(fields,
		"event", event,
		"phase", phase,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	fields = append(fields, attrs...)
	FromContext(ctx).InfoContext(ctx, "operation phase", fields...)
}
