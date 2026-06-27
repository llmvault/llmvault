package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func init() {
	RegisterTaskBuilder(TypeAgentTriggerStoreDelivery, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p AgentTriggerStoreDeliveryPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal agent trigger store delivery payload: %w", err)
		}
		return NewAgentTriggerStoreDeliveryTask(p)
	})
}

type AgentTriggerStoreDeliveryPayload struct {
	OrgID            uuid.UUID  `json:"org_id"`
	AgentID          uuid.UUID  `json:"agent_id"`
	TriggerID        uuid.UUID  `json:"trigger_id"`
	ConnectionID     *uuid.UUID `json:"connection_id,omitempty"`
	DeliveryID       string     `json:"delivery_id"`
	EventKey         string     `json:"event_key"`
	ResourceKey      string     `json:"resource_key"`
	SessionID        uuid.UUID  `json:"session_id"`
	RuntimeSessionID string     `json:"runtime_session_id"`
	RuntimeStreamID  string     `json:"runtime_stream_id"`
	RuntimeTraceID   string     `json:"runtime_trace_id"`
	RuntimeTurnID    string     `json:"runtime_turn_id"`
	PayloadJSON      []byte     `json:"payload"`
}

// NewAgentTriggerStoreDeliveryTask returns the task plus its enqueue options.
// Options are returned separately (see NewWebhookForwardTask).
func NewAgentTriggerStoreDeliveryTask(payload AgentTriggerStoreDeliveryPayload) (*asynq.Task, []asynq.Option, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent trigger store delivery payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Second),
	}
	return asynq.NewTask(TypeAgentTriggerStoreDelivery, encoded), opts, nil
}

type AgentTriggerStoreDeliveryHandler struct {
	db *gorm.DB
}

func NewAgentTriggerStoreDeliveryHandler(db *gorm.DB) *AgentTriggerStoreDeliveryHandler {
	return &AgentTriggerStoreDeliveryHandler{db: db}
}

func (h *AgentTriggerStoreDeliveryHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload AgentTriggerStoreDeliveryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	raw := payload.PayloadJSON
	if len(raw) == 0 {
		raw = []byte("{}")
	}

	row := model.AgentTriggerDelivery{
		OrgID:            payload.OrgID,
		AgentID:          payload.AgentID,
		TriggerID:        payload.TriggerID,
		ConnectionID:     payload.ConnectionID,
		DeliveryID:       payload.DeliveryID,
		EventKey:         payload.EventKey,
		ResourceKey:      payload.ResourceKey,
		SessionID:        payload.SessionID,
		RuntimeSessionID: payload.RuntimeSessionID,
		RuntimeStreamID:  payload.RuntimeStreamID,
		RuntimeTraceID:   payload.RuntimeTraceID,
		RuntimeTurnID:    payload.RuntimeTurnID,
		Payload:          model.RawJSON(raw),
	}
	// The dispatcher already claimed the (trigger_id, delivery_id) row, so upsert the runtime
	// correlation fields onto it; this also keeps the store task idempotent under asynq retries.
	db := h.db.WithContext(ctx)
	if strings.TrimSpace(payload.DeliveryID) != "" {
		db = db.Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "trigger_id"}, {Name: "delivery_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("delivery_id <> ?", "")}},
			DoUpdates: clause.AssignmentColumns([]string{
				"runtime_session_id", "runtime_stream_id", "runtime_trace_id", "runtime_turn_id",
				"resource_key", "event_key",
			}),
		})
	}
	if err := db.Create(&row).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("store agent trigger delivery: %w", err), triggerDeliverySentryFields(payload))
		return fmt.Errorf("store agent trigger delivery: %w", err)
	}
	return nil
}

func triggerDeliverySentryFields(payload AgentTriggerStoreDeliveryPayload) map[string]any {
	return map[string]any{
		"org_id":             payload.OrgID.String(),
		"agent_id":           payload.AgentID.String(),
		"trigger_id":         payload.TriggerID.String(),
		"delivery_id":        payload.DeliveryID,
		"event_key":          payload.EventKey,
		"resource_key":       payload.ResourceKey,
		"session_id":         payload.SessionID.String(),
		"runtime_session_id": payload.RuntimeSessionID,
	}
}
