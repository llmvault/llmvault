package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/sandbox"
)

const triggerConversationSource = "trigger"

func init() {
	RegisterTaskBuilder(TypeAgentTriggerDispatch, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p AgentTriggerDispatchPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal agent trigger dispatch payload: %w", err)
		}
		return NewAgentTriggerDispatchTask(p)
	})
}

type AgentTriggerDispatchHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
	catalog      *catalog.Catalog
	nangoClient  *nango.Client
}

func NewAgentTriggerDispatchHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, enqueuer ...enqueue.TaskEnqueuer) *AgentTriggerDispatchHandler {
	var q enqueue.TaskEnqueuer
	if len(enqueuer) > 0 {
		q = enqueuer[0]
	}
	return &AgentTriggerDispatchHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     q,
		catalog:      catalog.Global(),
	}
}

func (h *AgentTriggerDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload AgentTriggerDispatchPayload
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
		logging.CaptureWithFields(ctx, fmt.Errorf("agent trigger match failed: %w", err), map[string]any{
			"org_id":      payload.OrgID.String(),
			"delivery_id": payload.DeliveryID,
			"event_key":   eventKey(payload.EventType, payload.EventAction),
		})
		return err
	}
	for _, trigger := range triggers {
		// Curated triggers own their event handling end-to-end; the generic
		// catalog filters/conditions/prompt below do not apply to them.
		if isGitHubMentionTrigger(payload.Provider, trigger) {
			if err := h.deliverGitHubMention(ctx, payload, trigger, webhookPayload); err != nil {
				logging.CaptureWithFields(ctx, fmt.Errorf("deliver github mention trigger %s: %w", trigger.ID, err), map[string]any{
					"org_id":      payload.OrgID.String(),
					"agent_id":    trigger.AgentID.String(),
					"trigger_id":  trigger.ID.String(),
					"delivery_id": payload.DeliveryID,
					"event_key":   eventKey(payload.EventType, payload.EventAction),
				})
				return err
			}
			continue
		}
		skip, reason, err := h.shouldSkipTriggerDelivery(ctx, payload, webhookPayload)
		if err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("agent trigger filter failed: %w", err), map[string]any{
				"org_id":      payload.OrgID.String(),
				"delivery_id": payload.DeliveryID,
				"event_key":   eventKey(payload.EventType, payload.EventAction),
			})
			return err
		}
		if skip {
			logging.FromContext(ctx).InfoContext(ctx, "agent trigger skipped event",
				"trigger_id", trigger.ID,
				"agent_id", trigger.AgentID,
				"delivery_id", payload.DeliveryID,
				"event_key", eventKey(payload.EventType, payload.EventAction),
				"reason", reason)
			continue
		}
		if ok, reason := triggerConditionsMatch(trigger, webhookPayload); !ok {
			logging.FromContext(ctx).InfoContext(ctx, "agent trigger conditions skipped event",
				"trigger_id", trigger.ID, "agent_id", trigger.AgentID, "reason", reason)
			continue
		}
		if err := h.deliver(ctx, payload, trigger, webhookPayload); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("deliver agent trigger %s: %w", trigger.ID, err), map[string]any{
				"org_id":      payload.OrgID.String(),
				"agent_id":    trigger.AgentID.String(),
				"trigger_id":  trigger.ID.String(),
				"delivery_id": payload.DeliveryID,
				"event_key":   eventKey(payload.EventType, payload.EventAction),
			})
			return err
		}
	}
	return nil
}

func (h *AgentTriggerDispatchHandler) matchTriggers(ctx context.Context, payload AgentTriggerDispatchPayload) ([]model.AgentTrigger, error) {
	if payload.TriggerID != nil {
		var trigger model.AgentTrigger
		if err := h.db.WithContext(ctx).
			Preload("Agent").
			Where("id = ? AND org_id = ? AND enabled = true AND trigger_type = ?", *payload.TriggerID, payload.OrgID, "http").
			First(&trigger).Error; err != nil {
			return nil, fmt.Errorf("load http trigger: %w", err)
		}
		return []model.AgentTrigger{trigger}, nil
	}

	eventKeys := []string{eventKey(payload.EventType, payload.EventAction)}
	if payload.EventAction != "" {
		eventKeys = append(eventKeys, payload.EventType)
	}
	var triggers []model.AgentTrigger
	if err := h.db.WithContext(ctx).
		Joins("JOIN agents ON agents.id = agent_triggers.agent_id").
		Where("agent_triggers.org_id = ? AND agent_triggers.connection_id = ? AND agent_triggers.enabled = true AND agent_triggers.trigger_type = ? AND agents.status <> ?",
			payload.OrgID, payload.ConnectionID, "webhook", "archived").
		Where("agent_triggers.trigger_keys && ?", pq.StringArray(eventKeys)).
		Preload("Agent").
		Find(&triggers).Error; err != nil {
		return nil, fmt.Errorf("find agent triggers: %w", err)
	}
	return triggers, nil
}
