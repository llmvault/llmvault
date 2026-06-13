package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *AgentOutboundWebhookHandler) enqueueAgentMemoryRetain(ctx context.Context, sb *model.Sandbox, session *model.Session, sessionID, reason, sourceEvent string) {
	if h.enqueuer == nil || sb == nil || session == nil || sb.AgentID == nil || sessionID == "" {
		skipReason := agentMemoryRetainEnqueueSkipReason(h, sb, session, sessionID)
		logging.FromContext(ctx).WarnContext(ctx, "agent memory retain enqueue skipped",
			"skip_reason", skipReason,
			"session_id", sessionID,
			"reason", reason,
			"source_event", sourceEvent,
		)
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory retain enqueue skipped: %s", skipReason), agentMemoryRetainSentryFields(sb, session, sessionID, reason, sourceEvent))
		return
	}
	payload := tasks.AgentMemoryRetainPayload{
		AgentID:     *sb.AgentID,
		SandboxID:   sb.ID,
		SessionUUID: session.ID,
		SessionID:   sessionID,
		Reason:      reason,
		SourceEvent: sourceEvent,
	}
	duplicate, err := tasks.EnqueueAgentMemoryRetain(ctx, h.enqueuer, payload)
	if err != nil {
		captureAgentWebhookIngest(ctx, "enqueue_memory_retain", sb, nil, sessionID, session.Source, err)
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "agent memory retain enqueued",
			"org_id", firstUUIDString(sb.OrgID),
			"agent_id", sb.AgentID.String(),
			"sandbox_id", sb.ID.String(),
			"session_uuid", session.ID.String(),
			"runtime_session_id", sessionID,
			"source", session.Source,
			"reason", reason,
			"source_event", sourceEvent,
			"delay_seconds", int(tasks.AgentMemoryRetainDelay.Seconds()),
			"duplicate", duplicate,
		)
	}
}

func agentMemoryRetainSentryFields(sb *model.Sandbox, session *model.Session, sessionID, reason, sourceEvent string) map[string]any {
	fields := map[string]any{
		"runtime_session_id": sessionID,
		"reason":             reason,
		"source_event":       sourceEvent,
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
	if session != nil {
		fields["session_uuid"] = session.ID.String()
		fields["source"] = session.Source
	}
	return fields
}

func agentMemoryRetainEnqueueSkipReason(h *AgentOutboundWebhookHandler, sb *model.Sandbox, session *model.Session, sessionID string) string {
	switch {
	case h == nil || h.enqueuer == nil:
		return "enqueuer_missing"
	case sb == nil:
		return "sandbox_missing"
	case session == nil:
		return "session_missing"
	case sb.AgentID == nil:
		return "sandbox_agent_missing"
	case sessionID == "":
		return "runtime_session_id_missing"
	default:
		return "unknown"
	}
}

func firstUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
