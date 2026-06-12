package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/sandbox"
)

const triggerConversationSource = "trigger"

func init() {
	RegisterTaskBuilder(TypeEmployeeTriggerDispatch, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p EmployeeTriggerDispatchPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal employee trigger dispatch payload: %w", err)
		}
		return NewEmployeeTriggerDispatchTask(p)
	})
}

type EmployeeTriggerDispatchHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
	catalog      *catalog.Catalog
	nangoClient  *nango.Client
}

func NewEmployeeTriggerDispatchHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps employeeruntime.CompileDeps, enqueuer ...enqueue.TaskEnqueuer) *EmployeeTriggerDispatchHandler {
	var q enqueue.TaskEnqueuer
	if len(enqueuer) > 0 {
		q = enqueuer[0]
	}
	return &EmployeeTriggerDispatchHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     q,
		catalog:      catalog.Global(),
	}
}

func (h *EmployeeTriggerDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload EmployeeTriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	var webhookPayload map[string]any
	if len(payload.PayloadJSON) > 0 {
		if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
			return fmt.Errorf("decode trigger payload: %w", err)
		}
	}
	if webhookPayload == nil {
		webhookPayload = map[string]any{}
	}

	triggers, err := h.matchTriggers(ctx, payload)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger match failed: %w", err), map[string]any{
			"org_id":      payload.OrgID.String(),
			"delivery_id": payload.DeliveryID,
			"event_key":   eventKey(payload.EventType, payload.EventAction),
		})
		return err
	}
	for _, trigger := range triggers {
		skip, reason, err := h.shouldSkipTriggerDelivery(ctx, payload, webhookPayload)
		if err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger filter failed: %w", err), map[string]any{
				"org_id":      payload.OrgID.String(),
				"delivery_id": payload.DeliveryID,
				"event_key":   eventKey(payload.EventType, payload.EventAction),
			})
			return err
		}
		if skip {
			logging.FromContext(ctx).InfoContext(ctx, "employee trigger skipped event",
				"trigger_id", trigger.ID,
				"employee_id", trigger.EmployeeID,
				"delivery_id", payload.DeliveryID,
				"event_key", eventKey(payload.EventType, payload.EventAction),
				"reason", reason)
			continue
		}
		if ok, reason := triggerConditionsMatch(trigger, webhookPayload); !ok {
			logging.FromContext(ctx).InfoContext(ctx, "employee trigger conditions skipped event",
				"trigger_id", trigger.ID, "employee_id", trigger.EmployeeID, "reason", reason)
			continue
		}
		if err := h.deliver(ctx, payload, trigger, webhookPayload); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("deliver employee trigger %s: %w", trigger.ID, err), map[string]any{
				"org_id":      payload.OrgID.String(),
				"employee_id": trigger.EmployeeID.String(),
				"trigger_id":  trigger.ID.String(),
				"delivery_id": payload.DeliveryID,
				"event_key":   eventKey(payload.EventType, payload.EventAction),
			})
			return err
		}
	}
	return nil
}

func (h *EmployeeTriggerDispatchHandler) matchTriggers(ctx context.Context, payload EmployeeTriggerDispatchPayload) ([]model.EmployeeTrigger, error) {
	if payload.TriggerID != nil {
		var trigger model.EmployeeTrigger
		if err := h.db.WithContext(ctx).
			Preload("Employee").
			Where("id = ? AND org_id = ? AND enabled = true AND trigger_type = ?", *payload.TriggerID, payload.OrgID, "http").
			First(&trigger).Error; err != nil {
			return nil, fmt.Errorf("load http trigger: %w", err)
		}
		return []model.EmployeeTrigger{trigger}, nil
	}

	eventKeys := []string{eventKey(payload.EventType, payload.EventAction)}
	if payload.EventAction != "" {
		eventKeys = append(eventKeys, payload.EventType)
	}
	var triggers []model.EmployeeTrigger
	if err := h.db.WithContext(ctx).
		Joins("JOIN employees ON employees.id = employee_triggers.employee_id").
		Where("employee_triggers.org_id = ? AND employee_triggers.connection_id = ? AND employee_triggers.enabled = true AND employee_triggers.trigger_type = ? AND employees.status <> ?",
			payload.OrgID, payload.ConnectionID, "webhook", "archived").
		Where("employee_triggers.trigger_keys && ?", pq.StringArray(eventKeys)).
		Preload("Employee").
		Find(&triggers).Error; err != nil {
		return nil, fmt.Errorf("find employee triggers: %w", err)
	}
	return triggers, nil
}
