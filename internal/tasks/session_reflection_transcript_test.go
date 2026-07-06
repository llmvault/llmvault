package tasks

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestRenderSessionReflectionTranscriptIncludesActorsAndSkipsThinking(t *testing.T) {
	userID := uuid.New()
	session := model.Session{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		AgentID:   uuid.New(),
		Source:    model.SessionSourceWeb,
		CreatedBy: &userID,
	}
	webEvent := model.SessionEvent{
		ID:          uuid.New(),
		SessionID:   session.ID,
		EventType:   runtimeevents.EventUserMessageReceived,
		ActorUserID: &userID,
		Source:      model.SessionSourceWeb,
		Payload:     model.JSON{"text": "Please keep answers short."},
		EventAt:     time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	}
	thinking := model.SessionEvent{
		ID:        uuid.New(),
		SessionID: session.ID,
		EventType: runtimeevents.EventThinking,
		Payload:   model.JSON{"text": "hidden"},
		EventAt:   webEvent.EventAt.Add(time.Second),
	}
	slackEvent := model.SessionEvent{
		ID:        uuid.New(),
		SessionID: session.ID,
		EventType: runtimeevents.EventUserMessageReceived,
		Source:    model.SessionSourceExternal,
		Payload: model.JSON{
			"text": "Slack message:\n\nDeploy is blocked.",
			"slack": map[string]any{
				"team_id":    "T123",
				"channel_id": "C123",
				"thread_ts":  "1710000000.000000",
				"message_ts": "1710000001.000000",
				"sender_id":  "U123",
			},
		},
		EventAt: thinking.EventAt.Add(time.Second),
	}

	transcript, identities := renderSessionReflectionTranscript(session, "engineering", []model.SessionEvent{webEvent, thinking, slackEvent}, map[uuid.UUID]string{userID: "Dana"})
	if !strings.Contains(transcript, "Actor: Dana hivy_user="+userID.String()) {
		t.Fatalf("transcript missing Hivy actor:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Actor: <@U123>") || !strings.Contains(transcript, "Slack:") {
		t.Fatalf("transcript missing Slack actor/context:\n%s", transcript)
	}
	if strings.Contains(transcript, "hidden") || strings.Contains(transcript, runtimeevents.EventThinking) {
		t.Fatalf("transcript included thinking event:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Session Date: 2026-06-27") {
		t.Fatalf("transcript missing session date header:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Channel: engineering") {
		t.Fatalf("transcript missing channel header:\n%s", transcript)
	}
	if identities[webEvent.ID].UserID == nil || *identities[webEvent.ID].UserID != userID {
		t.Fatalf("web identity=%#v", identities[webEvent.ID])
	}
	if identities[slackEvent.ID].ExternalRef != "<@U123>" {
		t.Fatalf("slack identity=%#v", identities[slackEvent.ID])
	}
}

func TestRenderSessionReflectionTranscriptSummarizesToolResults(t *testing.T) {
	session := model.Session{
		ID:      uuid.New(),
		OrgID:   uuid.New(),
		AgentID: uuid.New(),
		Source:  model.SessionSourceWeb,
	}
	longOutput := strings.Repeat("total 48K drwxr-xr-x node_modules package.json ", 30)
	toolOK := model.SessionEvent{
		ID:        uuid.New(),
		SessionID: session.ID,
		EventType: runtimeevents.EventToolCallCompleted,
		Payload:   model.JSON{"tool_name": "bash", "text": longOutput},
		EventAt:   time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	}
	toolErr := model.SessionEvent{
		ID:        uuid.New(),
		SessionID: session.ID,
		EventType: runtimeevents.EventToolResult,
		Payload:   model.JSON{"tool_name": "web_fetch", "error": "connection refused"},
		EventAt:   toolOK.EventAt.Add(time.Second),
	}
	usage := model.SessionEvent{
		ID:        uuid.New(),
		SessionID: session.ID,
		EventType: runtimeevents.EventModelUsage,
		Payload:   model.JSON{"usage": map[string]any{"input_tokens": 100}},
		EventAt:   toolErr.EventAt.Add(time.Second),
	}

	transcript, _ := renderSessionReflectionTranscript(session, "", []model.SessionEvent{toolOK, toolErr, usage}, nil)
	if strings.Contains(transcript, longOutput) {
		t.Fatalf("transcript dumped full tool payload:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Result: bash ok — ") {
		t.Fatalf("transcript missing ok tool summary:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Result: web_fetch error — connection refused") {
		t.Fatalf("transcript missing error tool summary:\n%s", transcript)
	}
	if strings.Contains(transcript, runtimeevents.EventModelUsage) {
		t.Fatalf("transcript included model usage event:\n%s", transcript)
	}
	for _, line := range strings.Split(transcript, "\n") {
		if strings.HasPrefix(line, "Result: ") && len(line) > len("Result: bash ok — ")+reflectionTranscriptMaxSummary+8 {
			t.Fatalf("summary line too long (%d chars): %s", len(line), line)
		}
	}
	if !strings.Contains(transcript, "Session Date: 2026-06-27") {
		t.Fatalf("transcript missing session date header:\n%s", transcript)
	}
	if strings.Contains(transcript, "Channel:") {
		t.Fatalf("transcript should omit channel header when name empty:\n%s", transcript)
	}
}
