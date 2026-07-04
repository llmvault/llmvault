package tasks

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentTriggerDispatchHandler) deliver(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) error {
	agent, err := h.loadTriggerAgent(ctx, trigger)
	if err != nil {
		return err
	}
	compiled := h.compileMessage(payload, trigger, webhookPayload)
	return h.deliverCompiled(ctx, payload, trigger, agent, compiled)
}

func (h *AgentTriggerDispatchHandler) loadTriggerAgent(ctx context.Context, trigger model.AgentTrigger) (model.Agent, error) {
	agent := trigger.Agent
	if agent.ID == uuid.Nil {
		if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", trigger.AgentID, "archived").First(&agent).Error; err != nil {
			return model.Agent{}, fmt.Errorf("load agent: %w", err)
		}
	}
	if agent.OrgID == nil {
		return model.Agent{}, fmt.Errorf("agent missing org")
	}
	return agent, nil
}

func (h *AgentTriggerDispatchHandler) deliverCompiled(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, agent model.Agent, compiled compiledTriggerMessage) error {
	session, err := h.findOrCreateTriggerSession(ctx, &agent, trigger, compiled.ResourceKey)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "find_or_create_trigger_session", payload, trigger, compiled.ResourceKey, "", err)
		return err
	}

	// Claim before the durable enqueue: asynq retries the whole batch and
	// GitHub/Nango may redeliver, so a claimed row means this event was already
	// queued and its message row must not be created twice.
	claimed, err := h.claimTriggerDelivery(ctx, payload, trigger, session, compiled)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "claim_trigger_delivery", payload, trigger, compiled.ResourceKey, session.ID.String(), err)
		return err
	}
	if claimed {
		if err := h.enqueueTriggerSessionMessage(ctx, session, compiled); err != nil {
			// The message row was not created, so roll back the claim and let the
			// asynq retry re-queue it.
			h.releaseTriggerDeliveryClaim(ctx, trigger.ID, payload.DeliveryID)
			captureTriggerDispatchBoundary(ctx, "enqueue_trigger_session_message", payload, trigger, compiled.ResourceKey, session.ID.String(), err)
			return err
		}
		h.enqueueStoreDelivery(ctx, payload, trigger, session, compiled, nil)
	} else {
		logging.FromContext(ctx).InfoContext(ctx, "agent trigger already queued, re-triggering delivery",
			"trigger_id", trigger.ID,
			"agent_id", trigger.AgentID,
			"delivery_id", payload.DeliveryID,
			"event_key", eventKey(payload.EventType, payload.EventAction))
	}

	// Fire-and-forget: hand the message to the generic session delivery machinery,
	// which arbitrates the agent turn (deliver now when idle, after the active
	// turn otherwise) and provisions the runtime. We never wait for a reply — the
	// agent acts and responds through its own tools. Re-enqueuing is idempotent
	// (a no-op when nothing is pending), so it is safe on asynq retries and when
	// the delivery was already claimed by a prior attempt.
	if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, session.ID); err != nil {
		captureTriggerDispatchBoundary(ctx, "enqueue_session_message_deliver", payload, trigger, compiled.ResourceKey, session.ID.String(), err)
		return err
	}
	return nil
}

// enqueueTriggerSessionMessage appends the compiled trigger message to the
// session's message queue under a session lock so the (session_id,
// sequence_number) allocation is race-free. The generic session delivery worker
// drains it, arbitrating the agent turn — mid-turn triggers queue behind the
// active turn instead of colliding with it.
func (h *AgentTriggerDispatchHandler) enqueueTriggerSessionMessage(ctx context.Context, session *model.Session, compiled compiledTriggerMessage) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ?", session.ID).Error; err != nil {
			return fmt.Errorf("lock trigger session: %w", err)
		}
		seq, err := nextSessionQueueSequence(tx, locked.ID)
		if err != nil {
			return err
		}
		messagePayload := model.JSON{}
		maps.Copy(messagePayload, compiled.Raw)
		messagePayload["text"] = compiled.Text
		queue := model.SessionMessageQueue{
			OrgID:           locked.OrgID,
			SessionID:       locked.ID,
			MessageText:     compiled.Text,
			MessagePayload:  messagePayload,
			SequenceNumber:  seq,
			Status:          "pending",
			Model:           locked.Model,
			ReasoningEffort: locked.ReasoningEffort,
		}
		if err := tx.Create(&queue).Error; err != nil {
			return fmt.Errorf("create trigger message queue: %w", err)
		}
		return nil
	})
}

// claimTriggerDelivery inserts the (trigger_id, delivery_id) row before posting
// via ON CONFLICT DO NOTHING, returning true if this attempt won the claim and
// false if a prior attempt already delivered it.
func (h *AgentTriggerDispatchHandler) claimTriggerDelivery(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, session *model.Session, compiled compiledTriggerMessage) (bool, error) {
	if strings.TrimSpace(payload.DeliveryID) == "" {
		// No stable delivery id to dedupe on; fall back to always-deliver.
		return true, nil
	}
	raw := payload.PayloadJSON
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	row := model.AgentTriggerDelivery{
		OrgID:        trigger.OrgID,
		AgentID:      trigger.AgentID,
		TriggerID:    trigger.ID,
		ConnectionID: trigger.ConnectionID,
		DeliveryID:   payload.DeliveryID,
		EventKey:     eventKey(payload.EventType, payload.EventAction),
		ResourceKey:  compiled.ResourceKey,
		SessionID:    session.ID,
		Payload:      model.RawJSON(raw),
	}
	result := h.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "trigger_id"}, {Name: "delivery_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("delivery_id <> ?", "")}},
			DoNothing:   true,
		}).
		Create(&row)
	if result.Error != nil {
		return false, fmt.Errorf("claim trigger delivery: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// releaseTriggerDeliveryClaim removes a claim whose post failed so the asynq
// retry can re-deliver. It only deletes rows that have not yet recorded a
// runtime session id (i.e. were never confirmed delivered).
func (h *AgentTriggerDispatchHandler) releaseTriggerDeliveryClaim(ctx context.Context, triggerID uuid.UUID, deliveryID string) {
	if strings.TrimSpace(deliveryID) == "" {
		return
	}
	if err := h.db.WithContext(ctx).
		Where("trigger_id = ? AND delivery_id = ? AND runtime_session_id = ?", triggerID, deliveryID, "").
		Delete(&model.AgentTriggerDelivery{}).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("release trigger delivery claim: %w", err), map[string]any{
			"trigger_id":  triggerID.String(),
			"delivery_id": deliveryID,
		})
	}
}

func captureTriggerDispatchBoundary(ctx context.Context, stage string, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, resourceKey, sessionID string, err error) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("agent trigger dispatch %s: %w", stage, err), map[string]any{
		"stage":        stage,
		"org_id":       payload.OrgID.String(),
		"agent_id":     trigger.AgentID.String(),
		"trigger_id":   trigger.ID.String(),
		"delivery_id":  payload.DeliveryID,
		"event_key":    eventKey(payload.EventType, payload.EventAction),
		"resource_key": resourceKey,
		"session_id":   sessionID,
	})
}
