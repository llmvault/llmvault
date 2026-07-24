package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestRenderSessionReflectionTranscriptDoesNotLabelAgentOutputAsHuman(t *testing.T) {
	creatorID := uuid.New()
	session := model.Session{
		ID:        uuid.New(),
		AgentID:   uuid.New(),
		Source:    model.SessionSourceWeb,
		CreatedBy: &creatorID,
	}
	humanEvent := model.SessionEvent{
		ID:          uuid.New(),
		EventType:   runtimeevents.EventUserMessageReceived,
		ActorUserID: &creatorID,
		Payload:     model.JSON{"text": "Check which Slack channels are available."},
		EventAt:     time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC),
	}
	agentEvent := model.SessionEvent{
		ID:        uuid.New(),
		EventType: runtimeevents.EventFinal,
		Payload: model.JSON{
			"text": "I can see all-hive, social, engineering, and qa.",
		},
		EventAt: time.Date(2026, 7, 16, 10, 1, 0, 0, time.UTC),
	}

	transcript, identities := renderSessionReflectionTranscript(
		session,
		"",
		[]model.SessionEvent{humanEvent, agentEvent},
		map[uuid.UUID]string{creatorID: "Dana"},
	)

	humanBlock := reflectionTranscriptEventBlock(t, transcript, humanEvent.ID, agentEvent.ID)
	if !strings.Contains(humanBlock, "Role: Human") || !strings.Contains(humanBlock, "Actor: Dana") {
		t.Fatalf("human event missing role or actor:\n%s", humanBlock)
	}
	agentBlock := reflectionTranscriptEventBlock(t, transcript, agentEvent.ID, uuid.Nil)
	if !strings.Contains(agentBlock, "Role: Agent") {
		t.Fatalf("agent event missing agent role:\n%s", agentBlock)
	}
	if strings.Contains(agentBlock, "Actor: Dana") {
		t.Fatalf("agent event was mislabeled with the session creator:\n%s", agentBlock)
	}
	if identity := identities[agentEvent.ID]; identity.UserID != nil || identity.DisplayName != "" || identity.ExternalRef != "" {
		t.Fatalf("agent event retained human identity: %#v", identity)
	}
}

func reflectionTranscriptEventBlock(t *testing.T, transcript string, startID, nextID uuid.UUID) string {
	t.Helper()
	start := strings.Index(transcript, "[event:"+startID.String()+"]")
	if start < 0 {
		t.Fatalf("event %s missing from transcript:\n%s", startID, transcript)
	}
	end := len(transcript)
	if nextID != uuid.Nil {
		if next := strings.Index(transcript[start+1:], "[event:"+nextID.String()+"]"); next >= 0 {
			end = start + 1 + next
		}
	}
	return transcript[start:end]
}
