package specialisttasks

import (
	"context"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const (
	defaultTimelineLimit = 20
	maxTimelineLimit     = 100
)

func (s *Service) Timeline(ctx context.Context, token *model.Token, taskID uuid.UUID, limit, offset int) (*TaskTimelineResponse, *ToolError) {
	task, employee, toolErr := s.loadOwnedTask(ctx, token, taskID)
	if toolErr != nil {
		return nil, toolErr
	}
	if limit <= 0 {
		limit = defaultTimelineLimit
	}
	if limit > maxTimelineLimit {
		limit = maxTimelineLimit
	}
	if offset < 0 {
		offset = 0
	}
	var rows []model.EmployeeSessionEvent
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND specialist_task_id = ?", *employee.OrgID, employee.ID, task.ID).
		Order("event_at ASC, sequence_number ASC, id ASC").
		Limit(limit + 1).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, wrapToolError("event_load_failed", "Could not load specialist timeline events.", err, true, "Retry specialist_task_timeline. If it repeats, report that task events are unavailable.")
	}
	nextOffset := (*int)(nil)
	if len(rows) > limit {
		next := offset + limit
		nextOffset = &next
		rows = rows[:limit]
	}
	events := make([]SpecialistTimelineEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, SpecialistTimelineEvent{
			EventAt:   row.EventAt,
			EventType: row.EventType,
			Source:    row.Source,
			Summary:   compactText(payloadString(row.Payload, "text", "message", "content", "error", "output", "tool", "args", "result"), 1200),
		})
	}
	return &TaskTimelineResponse{
		TaskID:         task.ID.String(),
		SpecialistSlug: task.SpecialistSlug,
		Status:         task.Status,
		Limit:          limit,
		Offset:         offset,
		NextOffset:     nextOffset,
		Events:         events,
		NextAction:     "Use next_offset to fetch the next page. Remember: specialist sandboxes run on separate computers; ask specialists to upload files to Drive for shared artifacts.",
	}, nil
}
