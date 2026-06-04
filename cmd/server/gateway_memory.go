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

func enqueueGatewayEmployeeMemoryRetain(ctx context.Context, enqueuer enqueue.TaskEnqueuer, session model.EmployeeSession, reason, sourceEvent string) {
	if enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway employee memory retain: enqueuer missing"), gatewayEmployeeMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	if session.ID == uuid.Nil || session.EmployeeID == uuid.Nil || session.SandboxID == uuid.Nil {
		logging.FromContext(ctx).WarnContext(ctx, "gateway employee memory retain enqueue skipped",
			"skip_reason", "session_identity_missing",
			"employee_session_id", session.ID.String(),
			"employee_id", session.EmployeeID.String(),
			"sandbox_id", session.SandboxID.String(),
			"reason", reason,
			"source_event", sourceEvent,
		)
		return
	}
	payload := tasks.EmployeeMemoryRetainPayload{
		EmployeeID:        session.EmployeeID,
		SandboxID:         session.SandboxID,
		EmployeeSessionID: session.ID,
		SessionID:         session.RuntimeConversationID,
		Reason:            reason,
		SourceEvent:       sourceEvent,
	}
	duplicate, err := tasks.EnqueueEmployeeMemoryRetain(ctx, enqueuer, payload)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway employee memory retain: enqueue: %w", err), gatewayEmployeeMemoryRetainFields(session, reason, sourceEvent))
		return
	}
	logging.FromContext(ctx).InfoContext(ctx, "gateway employee memory retain enqueued",
		"org_id", session.OrgID.String(),
		"employee_id", session.EmployeeID.String(),
		"sandbox_id", session.SandboxID.String(),
		"employee_session_id", session.ID.String(),
		"runtime_conversation_id", session.RuntimeConversationID,
		"reason", reason,
		"source_event", sourceEvent,
		"delay_seconds", int(tasks.EmployeeMemoryRetainDelay.Seconds()),
		"duplicate", duplicate,
	)
}

func gatewayEmployeeMemoryRetainFields(session model.EmployeeSession, reason, sourceEvent string) map[string]any {
	return map[string]any{
		"org_id":                  session.OrgID.String(),
		"employee_id":             session.EmployeeID.String(),
		"sandbox_id":              session.SandboxID.String(),
		"employee_session_id":     session.ID.String(),
		"runtime_conversation_id": session.RuntimeConversationID,
		"reason":                  reason,
		"source_event":            sourceEvent,
	}
}
