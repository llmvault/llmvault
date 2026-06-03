package precontext

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestFormatSessionSummaryKeepsUserAndModelTextOnly(t *testing.T) {
	session := model.EmployeeSession{
		ID:                    uuid.New(),
		Name:                  "Slack thread",
		RuntimeConversationID: "runtime-1",
		UpdatedAt:             time.Date(2026, 6, 3, 12, 30, 0, 0, time.FixedZone("WAT", 3600)),
	}
	events := []model.EmployeeSessionEvent{
		{EventType: "tool.call.started", Payload: model.RawJSON(`{"text":"secret tool output"}`), EventAt: time.Now()},
		{EventType: "user.message.received", Payload: model.RawJSON(`{"text":"What tools do you have? token=abc123"}`), EventAt: time.Now()},
		{EventType: "agent.message.sent", Payload: model.RawJSON(`{"text":"I can search docs and remember project facts."}`), EventAt: time.Now()},
	}

	out := formatSessionSummary(session, events)
	if out == "" {
		t.Fatal("expected summary")
	}
	if containsAll(out, "tool", "secret") {
		t.Fatalf("included tool/internal noise: %s", out)
	}
	if !containsAll(out, "What tools do you have?", "I can search docs") {
		t.Fatalf("missing user/model text: %s", out)
	}
	if !strings.Contains(out, "2026-06-03T11:30:00Z") {
		t.Fatalf("missing session timestamp: %s", out)
	}
	if containsAll(out, "abc123") {
		t.Fatalf("did not redact token value: %s", out)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
