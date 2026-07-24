package logging

import (
	"context"
	"fmt"
	"time"

	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

// LogPhase emits a structured timing record for one step in a larger operation.
func LogPhase(ctx context.Context, event string, phase string, started time.Time, attrs ...any) {
	span := sentryobs.StartSpan(ctx, "provision.phase", event+" / "+phase)
	if span != nil {
		span.StartTime = started
		for i := 0; i+1 < len(attrs); i += 2 {
			key, ok := attrs[i].(string)
			if ok && key != "error" {
				span.SetData(key, attrs[i+1])
			}
		}
	}
	fields := make([]any, 0, len(attrs)+10)
	fields = append(fields,
		"event", event,
		"phase", phase,
		"status", "success",
		"duration_ms", time.Since(started).Milliseconds(),
	)
	if span != nil {
		fields = append(fields,
			"sentry_trace_id", fmt.Sprint(span.TraceID),
			"sentry_span_id", fmt.Sprint(span.SpanID),
		)
	}
	fields = append(fields, attrs...)
	FromContext(ctx).InfoContext(ctx, "operation phase", fields...)
	sentryobs.FinishSpanWithError(span, nil)
}
