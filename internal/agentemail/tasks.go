package agentemail

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeSend = "agent_email:send"

type SendTaskPayload struct {
	MessageID uuid.UUID `json:"message_id"`
}

func NewSendTask(messageID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	if messageID == uuid.Nil {
		return nil, nil, fmt.Errorf("message_id is required")
	}
	payload, err := json.Marshal(SendTaskPayload{MessageID: messageID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent email send payload: %w", err)
	}
	return asynq.NewTask(TypeSend, payload), []asynq.Option{asynq.Queue("critical"), asynq.MaxRetry(7), asynq.Timeout(2 * time.Minute)}, nil
}
