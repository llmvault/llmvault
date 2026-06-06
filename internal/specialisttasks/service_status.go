package specialisttasks

import (
	"context"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) Status(ctx context.Context, token *model.Token, taskID uuid.UUID) (*TaskStatusResponse, *ToolError) {
	task, employee, toolErr := s.loadOwnedTask(ctx, token, taskID)
	if toolErr != nil {
		return nil, toolErr
	}
	activity, toolErr := s.taskActivity(ctx, employee, task, 30)
	if toolErr != nil {
		return nil, toolErr
	}
	nextAction := "If the task is still running, wait and call specialist_task_status again. If more context is needed, call specialist_task_send_message with this task_id."
	if task.Status == "idle" {
		nextAction = "The specialist is idle and on standby. Review the latest specialist message; if more follow-up is needed, call specialist_task_send_message with this task_id."
	}
	return &TaskStatusResponse{
		TaskID:          task.ID.String(),
		SpecialistSlug:  task.SpecialistSlug,
		Status:          task.Status,
		CreatedAt:       task.CreatedAt,
		EndedAt:         task.EndedAt,
		LastActivityAt:  activity.LastActivityAt,
		ActivitySummary: activity.Summary(),
		LatestMessage:   activity.LatestMessage,
		LatestError:     activity.LatestError,
		NextAction:      nextAction,
	}, nil
}
