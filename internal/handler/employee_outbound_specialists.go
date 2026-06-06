package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeOutboundWebhookHandler) handleSpecialistAgentMessage(ctx context.Context, specialistSandbox *model.Sandbox, task *model.SpecialistTask, payload map[string]any, runtimeSessionID string) {
	if h == nil || h.db == nil || task == nil {
		return
	}
	transitioned, err := h.markSpecialistTaskIdle(ctx, task.ID)
	if err != nil {
		captureEmployeeWebhookIngest(ctx, "specialist_mark_idle", specialistSandbox, nil, runtimeSessionID, "specialist", err)
		return
	}
	if !transitioned {
		return
	}
	if err := h.notifyParentEmployeeSessionOfSpecialistIdle(ctx, task, payload); err != nil {
		captureEmployeeWebhookIngest(ctx, "specialist_notify_parent", specialistSandbox, nil, runtimeSessionID, "specialist", err)
	}
}

func (h *EmployeeOutboundWebhookHandler) markSpecialistTaskIdle(ctx context.Context, taskID uuid.UUID) (bool, error) {
	now := h.now().UTC()
	result := h.db.WithContext(ctx).Model(&model.SpecialistTask{}).
		Where("id = ? AND status = ?", taskID, "running").
		Updates(map[string]any{
			"status":     "idle",
			"updated_at": now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (h *EmployeeOutboundWebhookHandler) notifyParentEmployeeSessionOfSpecialistIdle(ctx context.Context, task *model.SpecialistTask, payload map[string]any) error {
	parentSession, err := h.loadSpecialistParentSession(ctx, task)
	if err != nil {
		return err
	}
	var parentSandbox model.Sandbox
	if err := h.db.WithContext(ctx).Where("id = ?", parentSession.SandboxID).First(&parentSandbox).Error; err != nil {
		return fmt.Errorf("load parent employee sandbox: %w", err)
	}
	if h.encKey == nil {
		return fmt.Errorf("runtime encryption key is not configured")
	}
	runtimeSecret, err := h.encKey.DecryptString(parentSandbox.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt parent runtime secret: %w", err)
	}
	client := employeeruntime.NewClient(parentSandbox.RuntimeURL, runtimeSecret)
	text := specialistIdleNotificationText(task, payload)
	resp, err := client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:           text,
		ConversationID: parentSession.RuntimeConversationID,
		User:           "hivy-control-plane",
		Raw: map[string]any{
			"source":             "specialist_task_idle",
			"specialist_task_id": task.ID.String(),
			"specialist_slug":    task.SpecialistSlug,
		},
	})
	if err != nil {
		return fmt.Errorf("send specialist idle notification to employee runtime: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "specialist task idle notification delivered to employee runtime",
		"specialist_task_id", task.ID.String(),
		"specialist_slug", task.SpecialistSlug,
		"employee_session_id", parentSession.ID.String(),
		"runtime_session_id", parentSession.RuntimeConversationID,
		"runtime_trace_id", resp.TraceID,
		"runtime_turn_id", resp.TurnID,
	)
	return nil
}

func (h *EmployeeOutboundWebhookHandler) loadSpecialistParentSession(ctx context.Context, task *model.SpecialistTask) (model.EmployeeSession, error) {
	var session model.EmployeeSession
	if task.ConversationID != nil && *task.ConversationID != uuid.Nil {
		if err := h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND employee_id = ?", *task.ConversationID, task.OrgID, task.EmployeeID).
			First(&session).Error; err != nil {
			return session, fmt.Errorf("load specialist parent session by id: %w", err)
		}
		return session, nil
	}
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND runtime_conversation_id = ?", task.OrgID, task.EmployeeID, task.EmployeeSessionID).
		Order("created_at DESC").
		First(&session).Error; err != nil {
		return session, fmt.Errorf("load specialist parent session by runtime id: %w", err)
	}
	if err := h.db.WithContext(ctx).Model(task).Update("conversation_id", session.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return session, fmt.Errorf("backfill specialist parent conversation id: %w", err)
	}
	return session, nil
}

func specialistIdleNotificationText(task *model.SpecialistTask, payload map[string]any) string {
	message := compactWebhookText(stringValue(payload, "text"), 1800)
	if message == "" {
		message = "The specialist finished its current turn without a final text message."
	}
	return fmt.Sprintf("Specialist %s is idle and on standby.\n\nLatest specialist message:\n%s\n\nIf more follow-up is needed, send feedback to the same specialist task.", task.SpecialistSlug, message)
}

func compactWebhookText(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}
