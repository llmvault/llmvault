package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

	// Claim this (trigger, delivery) before the irreversible PostHTTPMessage.
	// asynq retries the whole batch on any failure, so without this claim a retry
	// re-delivers triggers 1..N-1 that already succeeded — the agent would act
	// twice (open duplicate PRs, send duplicate messages). A claimed row means a
	// prior attempt already posted, so skip.
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
		// The post never reached the runtime, so release the claim to allow the
		// asynq retry to re-deliver this trigger. Earlier triggers in the batch
		// keep their claims and are not re-delivered.
		h.releaseTriggerDeliveryClaim(ctx, trigger.ID, payload.DeliveryID)
		captureTriggerDispatchBoundary(ctx, "post_http_message", payload, trigger, compiled.ResourceKey, conv.ID.String(), err)
		return fmt.Errorf("post employee trigger message: %w", err)
	}
	h.enqueueStoreDelivery(ctx, payload, trigger, conv, compiled, resp)
	return nil
}

// claimTriggerDelivery inserts the delivery row for (trigger_id, delivery_id)
// before the message is posted, using ON CONFLICT DO NOTHING against the unique
// index. It returns true if this attempt won the claim (and should post), false
// if a prior attempt already claimed it (already delivered, skip). Runtime
// correlation fields are filled in afterwards by the store-delivery task.
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
