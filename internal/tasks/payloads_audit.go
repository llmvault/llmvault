package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/model"
)

// AuditWritePayload is the payload for TypeAuditWrite tasks.
type AuditWritePayload struct {
	Entry model.AuditEntry `json:"entry"`
}

// NewAuditWriteTask creates a task that writes an audit log entry. Options are returned separately
// so they survive the enqueue client's Sentry trace-payload rewrite.
func NewAuditWriteTask(entry model.AuditEntry) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(AuditWritePayload{Entry: entry})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal audit payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Second),
	}
	return asynq.NewTask(TypeAuditWrite, payload), opts, nil
}

// GenerationWritePayload is the payload for TypeGenerationWrite tasks.
type GenerationWritePayload struct {
	Entry model.Generation `json:"entry"`
}

// NewGenerationWriteTask creates a task that writes a generation record.
// Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewGenerationWriteTask(entry model.Generation) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(GenerationWritePayload{Entry: entry})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal generation payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Second),
	}
	return asynq.NewTask(TypeGenerationWrite, payload), opts, nil
}
