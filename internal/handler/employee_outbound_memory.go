package handler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *EmployeeOutboundWebhookHandler) enqueueEmployeeMemoryRetain(ctx context.Context, sb *model.Sandbox, session *model.EmployeeSession, sessionID, reason, sourceEvent string) {
	if h.enqueuer == nil || sb == nil || session == nil || sb.EmployeeID == nil || sessionID == "" {
		logging.FromContext(ctx).WarnContext(ctx, "employee memory retain enqueue skipped",
			"skip_reason", employeeMemoryRetainEnqueueSkipReason(h, sb, session, sessionID),
			"session_id", sessionID,
			"reason", reason,
			"source_event", sourceEvent,
		)
		return
	}
	task, err := tasks.NewEmployeeMemoryRetainTask(tasks.EmployeeMemoryRetainPayload{
		EmployeeID:        *sb.EmployeeID,
		SandboxID:         sb.ID,
		EmployeeSessionID: session.ID,
		SessionID:         sessionID,
		Reason:            reason,
		SourceEvent:       sourceEvent,
	})
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "build_memory_retain_task", sb, nil, sessionID, session.Source, err)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.ProcessIn(10*time.Minute),
		asynq.TaskID("employee-memory-retain:"+session.ID.String()),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
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
			"delay_seconds", 600,
			"duplicate", errors.Is(err, asynq.ErrDuplicateTask),
		)
	}
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

func (h *EmployeeOutboundWebhookHandler) specialistTaskForSandbox(ctx context.Context, sandboxID uuid.UUID) (*model.SpecialistTask, bool) {
	var task model.SpecialistTask
	if err := h.db.WithContext(ctx).
		Where("sandbox_id = ?", sandboxID).
		Order("created_at DESC").
		First(&task).Error; err != nil {
		return nil, false
	}
	return &task, true
}
