package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// planItem is one step of the agent's current plan, as carried by the latest
// plan_updated transcript event. The runtime's vocabulary is exactly
// {pending, in_progress, completed} (sandboxes/runtime/crates/domain/src/plan.rs);
// only "completed" is terminal.
type planItem struct {
	Step   string
	Status string
}

func (p planItem) terminal() bool {
	return normalizePlanStatus(p.Status) == "completed"
}

// incompletePlanItems returns the non-terminal steps of the session's current
// plan. The plan is a full snapshot re-emitted on every update, so the latest
// plan_updated event holds the current state. An empty result means there is no
// plan or every step is completed — in both cases no reminder is warranted.
func (h *PlanTurnReminderHandler) incompletePlanItems(ctx context.Context, sessionID uuid.UUID) ([]planItem, error) {
	var event model.SessionEvent
	err := h.db.WithContext(ctx).
		Where("session_id = ? AND event_type = ?", sessionID, runtimeevents.EventPlanUpdated).
		Order("sequence_number DESC").
		First(&event).Error
	if err != nil {
		if strings.Contains(err.Error(), "record not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("load latest plan event: %w", err)
	}
	items := planItemsFromPayload(event.Payload)
	incomplete := make([]planItem, 0, len(items))
	for _, item := range items {
		if !item.terminal() {
			incomplete = append(incomplete, item)
		}
	}
	return incomplete, nil
}

func planItemsFromPayload(payload model.JSON) []planItem {
	raw, ok := payload["plan"].([]any)
	if !ok {
		return nil
	}
	items := make([]planItem, 0, len(raw))
	for _, entry := range raw {
		record, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		step := strings.TrimSpace(stringFromAny(record["step"]))
		if step == "" {
			continue
		}
		items = append(items, planItem{Step: step, Status: stringFromAny(record["status"])})
	}
	return items
}

func normalizePlanStatus(status string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(status)), "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if normalized == "complete" {
		return "completed"
	}
	return normalized
}

func stringFromAny(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

// planReminderText builds the injected <system_reminder> block. It is short and
// non-accusatory: it either nudges the agent to keep working or, if the work is
// genuinely finished, to update the plan so it reflects reality before stopping.
func planReminderText(incomplete []planItem) string {
	var b strings.Builder
	b.WriteString("<system_reminder>\n")
	fmt.Fprintf(&b, "Your plan still has %s that %s not marked completed:\n",
		pluralItems(len(incomplete)), pluralVerb(len(incomplete)))
	for _, item := range incomplete {
		fmt.Fprintf(&b, "- %s (%s)\n", item.Step, normalizePlanStatus(item.Status))
	}
	b.WriteString("\nOnly end your turn when the job is done. If there is still work to do, keep going. ")
	b.WriteString("If the work is genuinely complete, update the plan so its statuses reflect reality before you stop.\n")
	b.WriteString("</system_reminder>")
	return b.String()
}

func pluralItems(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

func pluralVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
