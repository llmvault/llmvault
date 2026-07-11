package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestListSubagentEventsReturnsCompletedChildHistoryWithPagination(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "dispatch subagents")
	parentSessionID := uuid.MustParse(created.Session.ID)
	childSessionID := "subagent-subagent-task-1783783908708-1"
	otherChildSessionID := "subagent-subagent-task-1783783908708-2"
	base := time.Date(2026, 7, 11, 16, 31, 48, 0, time.UTC)

	events := []model.SessionEvent{
		newSubagentHistoryEvent(fx, parentSessionID, "child-started", "subagent_started", 1, base, childSessionID),
		newSubagentHistoryEvent(fx, parentSessionID, "child-token", "token", 2, base.Add(time.Second), childSessionID),
		newSubagentHistoryEvent(fx, parentSessionID, "child-completed", "subagent_completed", 3, base.Add(2*time.Second), childSessionID),
		newSubagentHistoryEvent(fx, parentSessionID, "other-child-token", "token", 4, base.Add(3*time.Second), otherChildSessionID),
		{
			OrgID: fx.org.ID, SessionID: parentSessionID, AgentID: fx.agent.ID,
			EventID: "parent-token", EventType: "token", Source: "runtime", SequenceNumber: 5,
			Payload: model.JSON{"scope": "main", "text": "parent output"},
			EventAt: base.Add(4 * time.Second), CreatedAt: base.Add(4 * time.Second),
		},
	}
	if err := h.db.Create(&events).Error; err != nil {
		t.Fatalf("seed session events: %v", err)
	}

	first := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/subagents/"+childSessionID+"/events?limit=2", fx, fx.owner, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", first.Code, first.Body.String())
	}
	var firstPage subagentEventsPage
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if !firstPage.HasMore || firstPage.NextCursor == nil {
		t.Fatalf("first page pagination = %+v, want has_more and next_cursor", firstPage)
	}
	if got := eventIDs(firstPage.Data); len(got) != 2 || got[0] != "child-completed" || got[1] != "child-token" {
		t.Fatalf("first page event ids = %v", got)
	}

	second := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/subagents/"+childSessionID+"/events?limit=2&cursor="+*firstPage.NextCursor, fx, fx.owner, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", second.Code, second.Body.String())
	}
	var secondPage subagentEventsPage
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if secondPage.HasMore || secondPage.NextCursor != nil {
		t.Fatalf("second page pagination = %+v, want final page", secondPage)
	}
	if got := eventIDs(secondPage.Data); len(got) != 1 || got[0] != "child-started" {
		t.Fatalf("second page event ids = %v", got)
	}
}

func TestListSubagentEventsValidatesChildIDAndAuthorizesParent(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "private session")

	invalid := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/subagents/%20/events", fx, fx.owner, nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid child id status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	var invalidBody map[string]string
	if err := json.Unmarshal(invalid.Body.Bytes(), &invalidBody); err != nil {
		t.Fatalf("decode invalid child response: %v", err)
	}
	if invalidBody["error"] != "invalid child session id" {
		t.Fatalf("invalid child error=%q", invalidBody["error"])
	}

	denied := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/subagents/subagent-private/events", fx, fx.bystander, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unauthorized parent status=%d body=%s", denied.Code, denied.Body.String())
	}
}

type subagentEventsPage struct {
	Data       []sessionEventOut `json:"data"`
	NextCursor *string           `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

func newSubagentHistoryEvent(
	fx sessionFixture,
	parentSessionID uuid.UUID,
	eventID string,
	eventType string,
	sequence int64,
	at time.Time,
	childSessionID string,
) model.SessionEvent {
	return model.SessionEvent{
		OrgID: fx.org.ID, SessionID: parentSessionID, AgentID: fx.agent.ID,
		EventID: eventID, EventType: eventType, Source: "runtime", SequenceNumber: sequence,
		Payload: model.JSON{
			"scope": "subagent",
			"subagent": map[string]any{
				"job_id":            "subagent-task-1783783908708-1",
				"agent_name":        "codebase-explorer",
				"parent_session_id": parentSessionID.String(),
				"child_session_id":  childSessionID,
			},
		},
		EventAt: at, CreatedAt: at,
	}
}

func eventIDs(events []sessionEventOut) []string {
	ids := make([]string, len(events))
	for i, event := range events {
		ids[i] = event.EventID
	}
	return ids
}
