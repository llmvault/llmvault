package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const triggerSoftwareEngineeringSpecialistSlug = "software-engineering-specialist"

type triggerRecentSpecialistTasks struct {
	SpecialistSlug string
	Attached       bool
	Tasks          []triggerRecentSpecialistTask
}

type triggerRecentSpecialistTask struct {
	ID             uuid.UUID
	Status         string
	Brief          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastActivityAt *time.Time
	LastActivity   string
}

func (h *EmployeeTriggerDispatchHandler) loadRecentSoftwareEngineeringTasks(ctx context.Context, employee model.Employee) (triggerRecentSpecialistTasks, error) {
	out := triggerRecentSpecialistTasks{SpecialistSlug: triggerSoftwareEngineeringSpecialistSlug}
	if !employeeHasAttachedSpecialist(employee, triggerSoftwareEngineeringSpecialistSlug) {
		return out, nil
	}
	out.Attached = true
	if employee.OrgID == nil {
		return out, fmt.Errorf("employee missing org")
	}

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	var tasks []model.SpecialistTask
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND specialist_slug = ? AND created_at >= ?",
			*employee.OrgID, employee.ID, triggerSoftwareEngineeringSpecialistSlug, cutoff).
		Order("updated_at DESC, created_at DESC").
		Limit(50).
		Find(&tasks).Error; err != nil {
		return out, fmt.Errorf("load recent software engineering specialist tasks: %w", err)
	}
	if len(tasks) == 0 {
		return out, nil
	}

	out.Tasks = make([]triggerRecentSpecialistTask, 0, len(tasks))
	taskIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		out.Tasks = append(out.Tasks, triggerRecentSpecialistTask{
			ID:        task.ID,
			Status:    task.Status,
			Brief:     task.Brief,
			CreatedAt: task.CreatedAt,
			UpdatedAt: task.UpdatedAt,
		})
	}
	activities, err := h.loadLatestSpecialistTaskActivities(ctx, *employee.OrgID, employee.ID, taskIDs)
	if err != nil {
		return out, err
	}
	for i := range out.Tasks {
		if activity, ok := activities[out.Tasks[i].ID]; ok {
			out.Tasks[i].LastActivityAt = &activity.EventAt
			out.Tasks[i].LastActivity = formatTriggerTaskActivity(activity)
		}
	}
	return out, nil
}

func (h *EmployeeTriggerDispatchHandler) loadLatestSpecialistTaskActivities(ctx context.Context, orgID, employeeID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]model.EmployeeSessionEvent, error) {
	var events []model.EmployeeSessionEvent
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND specialist_task_id IN ?", orgID, employeeID, taskIDs).
		Order("event_at DESC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load recent specialist task activity: %w", err)
	}
	activities := map[uuid.UUID]model.EmployeeSessionEvent{}
	for _, event := range events {
		if event.SpecialistTaskID == nil {
			continue
		}
		if _, exists := activities[*event.SpecialistTaskID]; exists {
			continue
		}
		activities[*event.SpecialistTaskID] = event
	}
	return activities, nil
}

func employeeHasAttachedSpecialist(employee model.Employee, slug string) bool {
	for _, attached := range employee.AttachedSpecialists {
		if attached == slug {
			return true
		}
	}
	return false
}

func formatTriggerTaskActivity(event model.EmployeeSessionEvent) string {
	text := triggerPayloadText(event.Payload)
	if text == "" {
		return event.EventType
	}
	return event.EventType + ": " + compactTriggerText(text, 180)
}

func triggerPayloadText(payload model.RawJSON) string {
	var value any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return ""
	}
	return firstTriggerPayloadString(value, "text", "message", "content", "error", "reason", "tool", "name")
}

func firstTriggerPayloadString(value any, keys ...string) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range keys {
			if found, ok := typed[key]; ok {
				if text := firstTriggerPayloadString(found, keys...); text != "" {
					return text
				}
			}
		}
		for _, found := range typed {
			if text := firstTriggerPayloadString(found, keys...); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := firstTriggerPayloadString(item, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}

func compactTriggerText(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 1 {
		return text[:max]
	}
	return strings.TrimSpace(text[:max-1]) + "..."
}
