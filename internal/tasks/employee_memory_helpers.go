package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeMemoryRetainHandler) enqueueRefresh(ctx context.Context, agentID, sandboxID uuid.UUID) {
	if h.enqueuer == nil {
		logging.FromContext(ctx).WarnContext(ctx, "employee memory refresh enqueue skipped: enqueuer missing",
			"employee_id", agentID.String(),
			"sandbox_id", sandboxID.String(),
		)
		return
	}
	h.updateAgentMemoryRefreshStatus(ctx, agentID, "queued", "")
	task, err := NewEmployeeMemoryRefreshTask(EmployeeMemoryRefreshPayload{
		EmployeeID: agentID,
		SandboxID:  sandboxID,
		Reason:     "hindsight_retain",
	})
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task,
		asynq.Unique(2*time.Minute),
		asynq.TaskID("employee-memory-refresh:"+agentID.String()),
	); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee memory retain: enqueue refresh: %w", err), map[string]any{
			"employee_id": agentID.String(),
			"sandbox_id":  sandboxID.String(),
		})
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "employee memory refresh enqueued",
			"employee_id", agentID.String(),
			"sandbox_id", sandboxID.String(),
			"duplicate", errors.Is(err, asynq.ErrDuplicateTask),
		)
	}
}

func (h *EmployeeMemoryRetainHandler) updateAgentMemoryRefreshStatus(ctx context.Context, agentID uuid.UUID, status, message string) {
	if h == nil || h.db == nil || agentID == uuid.Nil {
		return
	}
	updates := map[string]any{
		"memory_refresh_status": status,
		"memory_refresh_error":  truncateMemoryRefreshError(message),
	}
	if err := h.db.WithContext(ctx).Model(&model.Employee{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee memory retain: update refresh status: %w", err))
	}
}

func employeeMemoryRetainFields(payload EmployeeMemoryRetainPayload) map[string]any {
	fields := map[string]any{
		"employee_id":         payload.EmployeeID.String(),
		"sandbox_id":          payload.SandboxID.String(),
		"employee_session_id": payload.EmployeeSessionID.String(),
		"runtime_session_id":  strings.TrimSpace(payload.SessionID),
		"reason":              strings.TrimSpace(payload.Reason),
		"source_event":        strings.TrimSpace(payload.SourceEvent),
	}
	return fields
}

func logEmployeeMemoryRetainSkip(ctx context.Context, fields map[string]any, reason string) {
	fields["skip_reason"] = strings.TrimSpace(reason)
	logging.FromContext(ctx).InfoContext(ctx, "employee memory retain skipped", fieldsToArgs(fields)...)
}

func countEmployeeMemoryCandidateEvents(events []model.EmployeeSessionEvent) int {
	count := 0
	for _, event := range events {
		if event.EventType == "user.message.received" || event.EventType == "agent.message.sent" {
			count++
		}
	}
	return count
}

func fieldsToArgs(fields map[string]any) []any {
	args := make([]any, 0, len(fields)*2)
	for key, value := range fields {
		args = append(args, key, value)
	}
	return args
}
