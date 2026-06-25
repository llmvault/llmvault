package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func eventTypes(events []model.SessionEvent) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.EventType
	}
	return out
}

func assertSessionListIDs(t *testing.T, rr *httptest.ResponseRecorder, want []string) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Data []sessionOut `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}
	got := make([]string, len(out.Data))
	for i, session := range out.Data {
		got[i] = session.ID
	}
	if len(got) != len(want) {
		t.Fatalf("session ids=%v, want=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("session ids=%v, want=%v", got, want)
		}
	}
}

func assertSessionNameTaskEnqueued(t *testing.T, h *sessionHarness, sessionID string) {
	t.Helper()

	for _, task := range h.enqueuer.Tasks() {
		if task.TypeName != tasks.TypeSessionName {
			continue
		}

		var payload tasks.SessionNamePayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("decode %s payload: %v", tasks.TypeSessionName, err)
		}
		if payload.SessionID.String() == sessionID {
			return
		}
	}

	t.Fatalf("expected %s task for session %s", tasks.TypeSessionName, sessionID)
}

func seedActivitySession(t *testing.T, h *sessionHarness, fx sessionFixture, createdBy uuid.UUID, participantIDs []uuid.UUID, activityAt time.Time) model.Session {
	t.Helper()
	session := model.Session{
		OrgID:           fx.org.ID,
		ChannelID:       fx.channel.ID,
		AgentID:         fx.agent.ID,
		CreatedBy:       &createdBy,
		Model:           "deepseek-v4-flash",
		ReasoningEffort: "high",
		Source:          "web",
		Name:            "activity-" + uuid.NewString()[:8],
		Status:          "active",
		CreatedAt:       activityAt.Add(-time.Minute),
		UpdatedAt:       activityAt.Add(-time.Minute),
	}
	if err := h.db.Create(&session).Error; err != nil {
		t.Fatalf("create activity session: %v", err)
	}
	for _, userID := range participantIDs {
		participant := model.SessionParticipant{
			SessionID: session.ID,
			UserID:    userID,
			Role:      "collaborator",
			CreatedAt: activityAt.Add(-time.Minute),
		}
		if err := h.db.Create(&participant).Error; err != nil {
			t.Fatalf("create activity participant: %v", err)
		}
	}
	event := model.SessionEvent{
		OrgID:            fx.org.ID,
		SessionID:        session.ID,
		AgentID:          fx.agent.ID,
		RuntimeSessionID: session.ID.String(),
		EventID:          "activity-" + uuid.NewString(),
		EventType:        "user.message",
		ActorUserID:      &createdBy,
		Source:           "web",
		SequenceNumber:   1,
		Payload:          model.JSON{"text": session.Name},
		EventAt:          activityAt,
		CreatedAt:        activityAt,
	}
	if err := h.db.Create(&event).Error; err != nil {
		t.Fatalf("create activity event: %v", err)
	}
	return session
}
