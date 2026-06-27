package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) waitForAgentRuntimeLive(ctx context.Context, sb *model.Sandbox) error {
	healthURL := strings.TrimRight(o.runtimeControlURL(sb.RuntimeURL), "/") + "/healthz"
	deadline := time.Now().Add(agentHealthTimeout)
	client := &http.Client{Timeout: agentHealthProbeTimeout}
	attempt := 0
	started := time.Now()

	logging.FromContext(ctx).InfoContext(ctx, "waiting for agent runtime",
		"sandbox_id", sb.ID,
		"health_url", healthURL,
		"timeout_ms", agentHealthTimeout.Milliseconds(),
		"interval_ms", agentHealthInterval.Milliseconds(),
	)
	for time.Now().Before(deadline) {
		attempt++
		attemptStarted := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		resp, doErr := client.Do(req)
		if doErr != nil {
			logging.FromContext(ctx).DebugContext(ctx, "agent runtime probe transport error",
				"sandbox_id", sb.ID,
				"attempt", attempt,
				"attempt_duration_ms", time.Since(attemptStarted).Milliseconds(),
				"total_ms", time.Since(started).Milliseconds(),
				"error", doErr,
			)
		} else {
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				logging.FromContext(ctx).InfoContext(ctx, "agent runtime live",
					"sandbox_id", sb.ID,
					"attempts", attempt,
					"attempt_duration_ms", time.Since(attemptStarted).Milliseconds(),
					"duration_ms", time.Since(started).Milliseconds(),
				)
				return nil
			}
			logging.FromContext(ctx).DebugContext(ctx, "agent runtime probe non-200",
				"sandbox_id", sb.ID,
				"attempt", attempt,
				"status", status,
				"attempt_duration_ms", time.Since(attemptStarted).Milliseconds(),
				"total_ms", time.Since(started).Milliseconds(),
			)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(agentHealthInterval):
		}
	}
	return fmt.Errorf("agent runtime not live within %s (%d attempts)", agentHealthTimeout, attempt)
}
