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

func (h *AgentMemoryRetainHandler) enqueueRefresh(ctx context.Context, agentID, sandboxID uuid.UUID) {
	if h.enqueuer == nil {
		logging.FromContext(ctx).WarnContext(ctx, "agent memory refresh enqueue skipped: enqueuer missing",
			"agent_id", agentID.String(),
			"sandbox_id", sandboxID.String(),
		)
		return
	}
	h.updateAgentMemoryRefreshStatus(ctx, agentID, "queued", "")
	task, opts, err := NewAgentMemoryRefreshTask(AgentMemoryRefreshPayload{
		AgentID:   agentID,
		SandboxID: sandboxID,
		Reason:    "hindsight_retain",
	})
	if err != nil {
		logging.Capture(ctx, err)
		return
	}
	opts = append(opts,
		asynq.Unique(2*time.Minute),
		asynq.TaskID("agent-memory-refresh:"+agentID.String()),
	)
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
		logging.CaptureWithFields(ctx, fmt.Errorf("agent memory retain: enqueue refresh: %w", err), map[string]any{
			"agent_id":   agentID.String(),
			"sandbox_id": sandboxID.String(),
		})
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "agent memory refresh enqueued",
			"agent_id", agentID.String(),
			"sandbox_id", sandboxID.String(),
			"duplicate", errors.Is(err, asynq.ErrDuplicateTask),
		)
	}
}

func (h *AgentMemoryRetainHandler) updateAgentMemoryRefreshStatus(ctx context.Context, agentID uuid.UUID, status, message string) {
	if h == nil || h.db == nil || agentID == uuid.Nil {
		return
	}
	updates := map[string]any{
		"memory_refresh_status": status,
		"memory_refresh_error":  truncateMemoryRefreshError(message),
	}
	if err := h.db.WithContext(ctx).Model(&model.Agent{}).Where("id = ?", agentID).Updates(updates).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("agent memory retain: update refresh status: %w", err))
	}
}

func agentMemoryRetainFields(payload AgentMemoryRetainPayload) map[string]any {
	fields := map[string]any{
		"agent_id":           payload.AgentID.String(),
		"sandbox_id":         payload.SandboxID.String(),
		"agent_session_id":   payload.AgentSessionID.String(),
		"runtime_session_id": strings.TrimSpace(payload.SessionID),
		"reason":             strings.TrimSpace(payload.Reason),
		"source_event":       strings.TrimSpace(payload.SourceEvent),
	}
	return fields
}

func logAgentMemoryRetainSkip(ctx context.Context, fields map[string]any, reason string) {
	fields["skip_reason"] = strings.TrimSpace(reason)
	logging.FromContext(ctx).InfoContext(ctx, "agent memory retain skipped", fieldsToArgs(fields)...)
}

func countAgentMemoryCandidateEvents(events []model.AgentSessionEvent) int {
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
