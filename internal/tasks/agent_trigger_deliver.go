package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *AgentTriggerDispatchHandler) deliver(ctx context.Context, payload AgentTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) error {
	agent := trigger.Agent
	if agent.ID == uuid.Nil {
		if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", trigger.AgentID, "archived").First(&agent).Error; err != nil {
			return fmt.Errorf("load agent: %w", err)
		}
	}
	if agent.OrgID == nil {
		return fmt.Errorf("agent missing org")
	}

	sb, err := h.loadAgentSandbox(ctx, agent.ID, *agent.OrgID)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "load_agent_sandbox", payload, trigger, "", "", err)
		return err
	}
	client, err := h.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "agent_runtime_client", payload, trigger, "", "", err)
		return err
	}
	if err := client.Readyz(ctx); err != nil {
		err = fmt.Errorf("agent runtime readyz: %w", err)
		captureTriggerDispatchBoundary(ctx, "agent_runtime_readyz", payload, trigger, "", "", err)
		return err
	}

	compiled := h.compileMessage(payload, trigger, webhookPayload)
	session, err := h.findOrCreateTriggerSession(ctx, &agent, sb, trigger, compiled.ResourceKey)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "find_or_create_trigger_session", payload, trigger, compiled.ResourceKey, "", err)
		return err
	}

	// Claim before the irreversible PostHTTPMessage: asynq retries the whole batch,
	// so without a claim a retry re-delivers already-succeeded triggers and the
	// agent acts twice. A claimed row means a prior attempt posted, so skip.
	claimed, err := h.claimTriggerDelivery(ctx, payload, trigger, session, compiled)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "claim_trigger_delivery", payload, trigger, compiled.ResourceKey, session.ID.String(), err)
		return err
	}
	if !claimed {
		logging.FromContext(ctx).InfoContext(ctx, "agent trigger already delivered, skipping",
			"trigger_id", trigger.ID,
			"agent_id", trigger.AgentID,
			"delivery_id", payload.DeliveryID,
			"event_key", eventKey(payload.EventType, payload.EventAction))
		return nil
	}

	resp, err := client.PostHTTPMessage(ctx, agentruntime.HTTPMessageRequest{
		Text:            compiled.Text,
		SessionID:       session.ID.String(),
		User:            "hivy-trigger",
		UserDisplayName: "Hivy Trigger",
	})
	if err != nil {
		// The post never reached the runtime, so release the claim for the asynq
		// retry; earlier triggers keep their claims and are not re-delivered.
		h.releaseTriggerDeliveryClaim(ctx, trigger.ID, payload.DeliveryID)
		captureTriggerDispatchBoundary(ctx, "post_session_message", payload, trigger, compiled.ResourceKey, session.ID.String(), err)
		return fmt.Errorf("post agent trigger message: %w", err)
	}
	h.enqueueStoreDelivery(ctx, payload, trigger, session, compiled, resp)
	return nil
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

func (h *AgentTriggerDispatchHandler) loadAgentSandbox(ctx context.Context, agentID, orgID uuid.UUID) (*model.Sandbox, error) {
	sb, err := agentRuntimeSelector(h.db, h.compileDeps).MainRuntime(ctx, orgID, agentID)
	if err != nil {
		return nil, fmt.Errorf("load agent sandbox: %w", err)
	}
	return sb, nil
}
