package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// EmployeeTriggerDispatchPayload carries one inbound trigger event to the
// employee trigger dispatcher. For webhook triggers, TriggerID is empty and the
// worker matches all employee triggers for the connection/event. For HTTP
// triggers, TriggerID is set and the worker delivers only that trigger.
type EmployeeTriggerDispatchPayload struct {
	Provider     string     `json:"provider,omitempty"`
	EventType    string     `json:"event_type,omitempty"`
	EventAction  string     `json:"event_action,omitempty"`
	DeliveryID   string     `json:"delivery_id"`
	OrgID        uuid.UUID  `json:"org_id"`
	ConnectionID uuid.UUID  `json:"connection_id,omitempty"`
	TriggerID    *uuid.UUID `json:"trigger_id,omitempty"`
	PayloadJSON  []byte     `json:"payload"`
}

// NewEmployeeTriggerDispatchTask returns the task plus its enqueue options.
// Options are returned separately (rather than baked into the task) so they
// survive the enqueue client's Sentry trace-payload rewrite — a task rebuilt via
// asynq.NewTask drops baked options, which would silently demote this
// critical-queue task to the default queue with default retry/timeout (P0-11).
func NewEmployeeTriggerDispatchTask(payload EmployeeTriggerDispatchPayload) (*asynq.Task, []asynq.Option, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee trigger dispatch payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeEmployeeTriggerDispatch, encoded), opts, nil
}

// ConversationNamePayload is the payload for TypeConversationName tasks.
// The worker loads everything else (conversation, agent, credential, first
// message) from the DB — we only need the ID.
type ConversationNamePayload struct {
	ConversationID uuid.UUID `json:"conversation_id"`
}

// NewConversationNameTask creates a task that generates a title for a
// conversation by calling the cheapest model available to the conversation's
// credential provider. Bulk queue — this is nice-to-have UX, not critical
// path. MaxRetry is 3: transient provider failures are common and the
// handler is idempotent (refuses to overwrite an already-set name).
// Options are returned separately so they survive the enqueue client's Sentry
// trace-payload rewrite (P0-11).
func NewConversationNameTask(conversationID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	encoded, err := json.Marshal(ConversationNamePayload{ConversationID: conversationID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal conversation name payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Second),
		asynq.Unique(5 * time.Minute),
	}
	return asynq.NewTask(TypeConversationName, encoded), opts, nil
}
