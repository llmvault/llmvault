package handler

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func captureAgentWebhookIngest(ctx context.Context, stage string, sb *model.Sandbox, event *agentOutboundEvent, sessionID, source string, err error) {
	if err == nil {
		return
	}
	fields := map[string]any{
		"stage":      stage,
		"session_id": sessionID,
		"source":     source,
	}
	if sb != nil {
		fields["sandbox_id"] = sb.ID.String()
		if sb.OrgID != nil {
			fields["org_id"] = sb.OrgID.String()
		}
		if sb.AgentID != nil {
			fields["agent_id"] = sb.AgentID.String()
		}
	}
	if event != nil {
		fields["event_type"] = event.EventType
		if !event.At.IsZero() {
			fields["event_at"] = event.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("agent outbound webhook ingest %s: %w", stage, err), fields)
}

func captureSessionEventFailure(ctx context.Context, stage string, entry model.SessionEvent, err error) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("session event %s: %w", stage, err), sessionEventSentryFields(stage, entry))
}

func sessionEventSentryFields(stage string, entry model.SessionEvent) map[string]any {
	return map[string]any{
		"stage":      stage,
		"org_id":     entry.OrgID.String(),
		"agent_id":   entry.AgentID.String(),
		"sandbox_id": firstUUIDString(entry.SandboxID),
		"session_id": entry.SessionID.String(),
		"event_type": entry.EventType,
		"source":     entry.Source,
	}
}
