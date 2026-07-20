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
	transportErrors := 0
	non200Responses := 0
	firstHTTPResponseMS := int64(-1)
	lastStatus := 0
	lastTransportError := ""

	logging.FromContext(ctx).InfoContext(ctx, "waiting for agent runtime",
		"sandbox_id", sb.ID,
		"external_id", sb.ExternalID,
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
			transportErrors++
			lastTransportError = doErr.Error()
			logging.FromContext(ctx).DebugContext(ctx, "agent runtime probe transport error",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"attempt", attempt,
				"attempt_duration_ms", time.Since(attemptStarted).Milliseconds(),
				"total_ms", time.Since(started).Milliseconds(),
				"error", doErr,
			)
		} else {
			if firstHTTPResponseMS < 0 {
				firstHTTPResponseMS = time.Since(started).Milliseconds()
			}
			status := resp.StatusCode
			lastStatus = status
			resp.Body.Close()
			if status == http.StatusOK {
				logging.FromContext(ctx).InfoContext(ctx, "agent runtime live",
					"sandbox_id", sb.ID,
					"external_id", sb.ExternalID,
					"attempts", attempt,
					"transport_errors", transportErrors,
					"non_200_responses", non200Responses,
					"first_http_response_ms", firstHTTPResponseMS,
					"attempt_duration_ms", time.Since(attemptStarted).Milliseconds(),
					"duration_ms", time.Since(started).Milliseconds(),
				)
				return nil
			}
			non200Responses++
			logging.FromContext(ctx).DebugContext(ctx, "agent runtime probe non-200",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"attempt", attempt,
				"status", status,
				"attempt_duration_ms", time.Since(attemptStarted).Milliseconds(),
				"total_ms", time.Since(started).Milliseconds(),
			)
		}
		select {
		case <-ctx.Done():
			logging.FromContext(ctx).WarnContext(context.WithoutCancel(ctx), "agent runtime readiness canceled",
				"sandbox_id", sb.ID, "external_id", sb.ExternalID, "attempts", attempt, "transport_errors", transportErrors,
				"non_200_responses", non200Responses, "first_http_response_ms", firstHTTPResponseMS,
				"last_status", lastStatus, "last_transport_error", lastTransportError,
				"duration_ms", time.Since(started).Milliseconds())
			return ctx.Err()
		case <-time.After(agentHealthInterval):
		}
	}
	logging.FromContext(ctx).WarnContext(context.WithoutCancel(ctx), "agent runtime readiness timed out",
		"sandbox_id", sb.ID, "external_id", sb.ExternalID, "attempts", attempt, "transport_errors", transportErrors,
		"non_200_responses", non200Responses, "first_http_response_ms", firstHTTPResponseMS,
		"last_status", lastStatus, "last_transport_error", lastTransportError,
		"duration_ms", time.Since(started).Milliseconds())
	return fmt.Errorf("agent runtime not live within %s (%d attempts, %d transport errors, %d non-200 responses)",
		agentHealthTimeout, attempt, transportErrors, non200Responses)
}
