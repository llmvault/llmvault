package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *EmployeeOutboundWebhookHandler) enqueueEmployeeMemoryRetain(ctx context.Context, sb *model.Sandbox, session *model.EmployeeSession, sessionID, reason, sourceEvent string) {
	if h.enqueuer == nil || sb == nil || session == nil || sb.EmployeeID == nil || sessionID == "" {
		skipReason := employeeMemoryRetainEnqueueSkipReason(h, sb, session, sessionID)
		logging.FromContext(ctx).WarnContext(ctx, "employee memory retain enqueue skipped",
			"skip_reason", skipReason,
			"session_id", sessionID,
			"reason", reason,
			"source_event", sourceEvent,
		)
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory retain enqueue skipped: %s", skipReason), employeeMemoryRetainSentryFields(sb, session, sessionID, reason, sourceEvent))
		return
	}
	payload := tasks.EmployeeMemoryRetainPayload{
		EmployeeID:        *sb.EmployeeID,
		SandboxID:         sb.ID,
		EmployeeSessionID: session.ID,
		SessionID:         sessionID,
		Reason:            reason,
		SourceEvent:       sourceEvent,
	}
	duplicate, err := tasks.EnqueueEmployeeMemoryRetain(ctx, h.enqueuer, payload)
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "enqueue_memory_retain", sb, nil, sessionID, session.Source, err)
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "employee memory retain enqueued",
			"org_id", firstUUIDString(sb.OrgID),
			"employee_id", sb.EmployeeID.String(),
			"sandbox_id", sb.ID.String(),
			"employee_session_id", session.ID.String(),
			"runtime_session_id", sessionID,
			"source", session.Source,
			"reason", reason,
			"source_event", sourceEvent,
			"delay_seconds", int(tasks.EmployeeMemoryRetainDelay.Seconds()),
			"duplicate", duplicate,
		)
	}
}

func employeeMemoryRetainSentryFields(sb *model.Sandbox, session *model.EmployeeSession, sessionID, reason, sourceEvent string) map[string]any {
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
		if sb.EmployeeID != nil {
			fields["employee_id"] = sb.EmployeeID.String()
		}
	}
	if session != nil {
		fields["employee_session_id"] = session.ID.String()
		fields["source"] = session.Source
	}
	return fields
}

func employeeMemoryRetainEnqueueSkipReason(h *EmployeeOutboundWebhookHandler, sb *model.Sandbox, session *model.EmployeeSession, sessionID string) string {
	switch {
	case h == nil || h.enqueuer == nil:
		return "enqueuer_missing"
	case sb == nil:
		return "sandbox_missing"
	case session == nil:
		return "session_missing"
	case sb.EmployeeID == nil:
		return "sandbox_employee_missing"
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
