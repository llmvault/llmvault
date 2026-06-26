package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestListSubagentEventsFiltersByChildSession(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "dispatch subagents")
	sessionID := uuid.MustParse(created.Session.ID)
	base := time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC)

	events := []model.SessionEvent{
		subagentEvent(fx, sessionID, "child-a-start", "subagent_started", 1, base, model.JSON{
			"child_session_id": "child-a",
			"agent_name":       "codebase-reader",
		}),
		subagentEvent(fx, sessionID, "child-a-token", "token", 2, base.Add(time.Second), model.JSON{
			"scope": "subagent",
			"subagent": map[string]any{
				"child_session_id": "child-a",
				"job_id":           "job-a",
			},
			"text": "child a output",
		}),
		subagentEvent(fx, sessionID, "child-b-token", "token", 3, base.Add(2*time.Second), model.JSON{
			"scope": "subagent",
			"subagent": map[string]any{
				"child_session_id": "child-b",
				"job_id":           "job-b",
			},
			"text": "child b output",
		}),
		subagentEvent(fx, sessionID, "parent-token", "token", 4, base.Add(3*time.Second), model.JSON{
			"text": "parent output",
		}),
	}
	if err := h.db.Create(&events).Error; err != nil {
		t.Fatalf("seed subagent events: %v", err)
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/subagents/child-a/events?limit=10", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list subagent events status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Data []sessionEventOut `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode subagent events: %v\n%s", err, rr.Body.String())
	}
	if len(out.Data) != 2 {
		t.Fatalf("subagent events count=%d, want 2: %#v", len(out.Data), out.Data)
	}
	if out.Data[0].EventID != "child-a-token" || out.Data[1].EventID != "child-a-start" {
		t.Fatalf("subagent event ids=%q,%q", out.Data[0].EventID, out.Data[1].EventID)
	}
}

func subagentEvent(
	fx sessionFixture,
	sessionID uuid.UUID,
	eventID string,
	eventType string,
	sequence int64,
	at time.Time,
	payload model.JSON,
) model.SessionEvent {
	return model.SessionEvent{
		OrgID:            fx.org.ID,
		SessionID:        sessionID,
		AgentID:          fx.agent.ID,
		RuntimeSessionID: sessionID.String(),
		EventID:          eventID,
		EventType:        eventType,
		Source:           "runtime",
		SequenceNumber:   sequence,
		Payload:          payload,
		EventAt:          at,
		CreatedAt:        at,
	}
}
