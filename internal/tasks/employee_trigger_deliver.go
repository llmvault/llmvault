package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func (h *EmployeeTriggerDispatchHandler) deliver(ctx context.Context, payload EmployeeTriggerDispatchPayload, trigger model.EmployeeTrigger, webhookPayload map[string]any) error {
	agent := trigger.Employee
	if agent.ID == uuid.Nil {
		if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", trigger.EmployeeID, "archived").First(&agent).Error; err != nil {
			return fmt.Errorf("load employee: %w", err)
		}
	}
	if agent.OrgID == nil {
		return fmt.Errorf("employee missing org")
	}

	sb, err := h.loadEmployeeSandbox(ctx, agent.ID, *agent.OrgID)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "load_employee_sandbox", payload, trigger, "", "", err)
		return err
	}
	if strings.EqualFold(sb.Status, string(sandbox.StatusStopped)) {
		if err := h.orchestrator.StartEmployeeSandbox(ctx, sb); err != nil {
			captureTriggerDispatchBoundary(ctx, "start_employee_sandbox", payload, trigger, "", "", err)
			return err
		}
	} else if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			captureTriggerDispatchBoundary(ctx, "refresh_employee_sandbox_url", payload, trigger, "", "", err)
			return err
		}
	}

	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "decrypt_employee_runtime_key", payload, trigger, "", "", err)
		return fmt.Errorf("decrypt employee runtime key: %w", err)
	}
	client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		captureTriggerDispatchBoundary(ctx, "employee_runtime_healthz", payload, trigger, "", "", err)
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		if err := h.syncRuntime(ctx, &agent, sb, client); err != nil {
			captureTriggerDispatchBoundary(ctx, "employee_runtime_readyz_sync", payload, trigger, "", "", err)
			return err
		}
	}

	recentTasks, err := h.loadRecentSoftwareEngineeringTasks(ctx, agent)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "load_recent_software_engineering_tasks", payload, trigger, "", "", err)
		return err
	}
	compiled := h.compileMessage(payload, trigger, webhookPayload, recentTasks)
	conv, err := h.findOrCreateTriggerConversation(ctx, &agent, sb, trigger.ID, compiled.ResourceKey, compiled.ConversationID)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "find_or_create_trigger_conversation", payload, trigger, compiled.ResourceKey, "", err)
		return err
	}

	// Claim before the irreversible PostHTTPMessage: asynq retries the whole batch,
	// so without a claim a retry re-delivers already-succeeded triggers and the
	// agent acts twice. A claimed row means a prior attempt posted, so skip.
	claimed, err := h.claimTriggerDelivery(ctx, payload, trigger, conv, compiled)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "claim_trigger_delivery", payload, trigger, compiled.ResourceKey, conv.ID.String(), err)
		return err
	}
	if !claimed {
		logging.FromContext(ctx).InfoContext(ctx, "employee trigger already delivered, skipping",
			"trigger_id", trigger.ID,
			"employee_id", trigger.EmployeeID,
			"delivery_id", payload.DeliveryID,
			"event_key", eventKey(payload.EventType, payload.EventAction))
		return nil
	}

	resp, err := client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:            compiled.Text,
		ConversationID:  conv.RuntimeConversationID,
		User:            "hivy-trigger",
		UserDisplayName: "Hivy Trigger",
		Raw:             compiled.Raw,
	})
	if err != nil {
		// The post never reached the runtime, so release the claim for the asynq
		// retry; earlier triggers keep their claims and are not re-delivered.
		h.releaseTriggerDeliveryClaim(ctx, trigger.ID, payload.DeliveryID)
		captureTriggerDispatchBoundary(ctx, "post_http_message", payload, trigger, compiled.ResourceKey, conv.ID.String(), err)
		return fmt.Errorf("post employee trigger message: %w", err)
	}
	h.enqueueStoreDelivery(ctx, payload, trigger, conv, compiled, resp)
	return nil
}

// claimTriggerDelivery inserts the (trigger_id, delivery_id) row before posting
// via ON CONFLICT DO NOTHING, returning true if this attempt won the claim and
// false if a prior attempt already delivered it.
func (h *EmployeeTriggerDispatchHandler) claimTriggerDelivery(ctx context.Context, payload EmployeeTriggerDispatchPayload, trigger model.EmployeeTrigger, conv *model.EmployeeSession, compiled compiledTriggerMessage) (bool, error) {
	if strings.TrimSpace(payload.DeliveryID) == "" {
		// No stable delivery id to dedupe on; fall back to always-deliver.
		return true, nil
	}
	raw := payload.PayloadJSON
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	row := model.EmployeeTriggerDelivery{
		OrgID:                 trigger.OrgID,
		EmployeeID:            trigger.EmployeeID,
		TriggerID:             trigger.ID,
		ConnectionID:          trigger.ConnectionID,
		DeliveryID:            payload.DeliveryID,
		EventKey:              eventKey(payload.EventType, payload.EventAction),
		ResourceKey:           compiled.ResourceKey,
		ConversationID:        conv.ID,
		RuntimeConversationID: conv.RuntimeConversationID,
		Payload:               model.RawJSON(raw),
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
func (h *EmployeeTriggerDispatchHandler) releaseTriggerDeliveryClaim(ctx context.Context, triggerID uuid.UUID, deliveryID string) {
	if strings.TrimSpace(deliveryID) == "" {
		return
	}
	if err := h.db.WithContext(ctx).
		Where("trigger_id = ? AND delivery_id = ? AND runtime_session_id = ?", triggerID, deliveryID, "").
		Delete(&model.EmployeeTriggerDelivery{}).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("release trigger delivery claim: %w", err), map[string]any{
			"trigger_id":  triggerID.String(),
			"delivery_id": deliveryID,
		})
	}
}

func captureTriggerDispatchBoundary(ctx context.Context, stage string, payload EmployeeTriggerDispatchPayload, trigger model.EmployeeTrigger, resourceKey, conversationID string, err error) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger dispatch %s: %w", stage, err), map[string]any{
		"stage":           stage,
		"org_id":          payload.OrgID.String(),
		"employee_id":     trigger.EmployeeID.String(),
		"trigger_id":      trigger.ID.String(),
		"delivery_id":     payload.DeliveryID,
		"event_key":       eventKey(payload.EventType, payload.EventAction),
		"resource_key":    resourceKey,
		"conversation_id": conversationID,
	})
}

func (h *EmployeeTriggerDispatchHandler) loadEmployeeSandbox(ctx context.Context, agentID, orgID uuid.UUID) (*model.Sandbox, error) {
	sb, err := employeeRuntimeSelector(h.db, h.compileDeps).MainRuntime(ctx, orgID, agentID)
	if err != nil {
		return nil, fmt.Errorf("load employee sandbox: %w", err)
	}
	return sb, nil
}

func (h *EmployeeTriggerDispatchHandler) syncRuntime(ctx context.Context, agent *model.Employee, sb *model.Sandbox, client *employeeruntime.Client) error {
	runtimeSecret, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	configUpdate, _, err := employeeruntime.BuildEmployeeRuntimeConfigUpdate(ctx, h.compileDeps, agent, sb, runtimeSecret)
	if err != nil {
		return fmt.Errorf("build employee runtime config: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	return nil
}
