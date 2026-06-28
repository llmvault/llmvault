package tasks

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestNewSlackReactionTriggerTask(t *testing.T) {
	eventID := uuid.New()
	task, opts, err := NewSlackReactionTriggerTask(SlackReactionTriggerPayload{SlackThreadEventID: eventID}, "EvReaction")
	if err != nil {
		t.Fatalf("NewSlackReactionTriggerTask error: %v", err)
	}
	if task.Type() != TypeSlackReactionTrigger {
		t.Fatalf("task type=%q", task.Type())
	}
	if len(opts) == 0 {
		t.Fatal("expected task options")
	}
	var payload SlackReactionTriggerPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.SlackThreadEventID != eventID {
		t.Fatalf("payload event id=%s", payload.SlackThreadEventID)
	}
}
