package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func enqueueGatewayAgentMemoryRetain(ctx context.Context, enqueuer enqueue.TaskEnqueuer, session model.AgentSession, reason, sourceEvent string) {
	if enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway agent memory retain: enqueuer missing"), gatewayAgentMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	if session.ID == uuid.Nil || session.AgentID == uuid.Nil || session.SandboxID == uuid.Nil {
		logging.FromContext(ctx).WarnContext(ctx, "gateway agent memory retain enqueue skipped",
			"skip_reason", "session_identity_missing",
			"agent_session_id", session.ID.String(),
			"agent_id", session.AgentID.String(),
			"sandbox_id", session.SandboxID.String(),
			"reason", reason,
			"source_event", sourceEvent,
		)
		return
	}
	payload := tasks.AgentMemoryRetainPayload{
		AgentID:        session.AgentID,
		SandboxID:      session.SandboxID,
		AgentSessionID: session.ID,
		SessionID:      session.RuntimeConversationID,
		Reason:         reason,
		SourceEvent:    sourceEvent,
	}
	duplicate, err := tasks.EnqueueAgentMemoryRetain(ctx, enqueuer, payload)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway agent memory retain: enqueue: %w", err), gatewayAgentMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "gateway agent memory retain enqueued",
		"org_id", session.OrgID.String(),
		"agent_id", session.AgentID.String(),
		"sandbox_id", session.SandboxID.String(),
		"agent_session_id", session.ID.String(),
		"runtime_conversation_id", session.RuntimeConversationID,
		"reason", reason,
		"source_event", sourceEvent,
		"delay_seconds", int(tasks.AgentMemoryRetainDelay.Seconds()),
		"duplicate", duplicate,
	)
}

func gatewayAgentMemoryRetainFields(session model.AgentSession, reason, sourceEvent string) map[string]any {
	return map[string]any{
		"org_id":                  session.OrgID.String(),
		"agent_id":                session.AgentID.String(),
		"sandbox_id":              session.SandboxID.String(),
		"agent_session_id":        session.ID.String(),
		"runtime_conversation_id": session.RuntimeConversationID,
		"reason":                  reason,
		"source_event":            sourceEvent,
	}
}
