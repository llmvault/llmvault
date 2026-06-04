package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *employeeHarness) listEmployeeSessions(t *testing.T, m orgWithMember, agentID uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/employees/"+agentID.String()+"/sessions"+query, nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *employeeHarness) listEmployeeSessionEvents(t *testing.T, m orgWithMember, agentID, sessionID uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/employees/"+agentID.String()+"/sessions/"+sessionID.String()+"/events"+query, nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestEmployeeHandler_ListSessionsReturnsPersistedSessionStats(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	sb := h.seedSandbox(t, m, agent.ID)
	now := time.Now().UTC().Truncate(time.Second)
	session := seedEmployeeSession(t, h, m.org.ID, agent.ID, sb.ID, "slack", "Deploy question", now.Add(-2*time.Hour))
	seedEmployeeSessionEvent(t, h, session, "user.message.received", `{"text":"Can you check the deploy?"}`, now.Add(-90*time.Minute), 1)
	seedEmployeeSessionEvent(t, h, session, "agent.message.sent", `{"text":"The deploy is green."}`, now.Add(-80*time.Minute), 2)
	other := seedEmployeeSession(t, h, m.org.ID, agent.ID, sb.ID, "slack", "Other", now.Add(-time.Hour))
	seedEmployeeSessionEvent(t, h, other, "user.message.received", `{"text":"Other"}`, now.Add(-50*time.Minute), 1)

	rr := h.listEmployeeSessions(t, m, agent.ID, "?q=deploy")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Source         string `json:"source"`
			EventCount     int64  `json:"event_count"`
			LastActivityAt string `json:"last_activity_at"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("data len = %d, want 1: %#v", len(resp.Data), resp.Data)
	}
	item := resp.Data[0]
	if item.ID != session.ID.String() || item.Name != "Deploy question" || item.Source != "slack" || item.EventCount != 2 {
		t.Fatalf("unexpected session item: %#v", item)
	}
	if item.LastActivityAt == "" {
		t.Fatal("last_activity_at is empty")
	}
}

func TestEmployeeHandler_ListSessionEventsReturnsCursorPaginatedPayloads(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	sb := h.seedSandbox(t, m, agent.ID)
	now := time.Now().UTC().Truncate(time.Second)
	session := seedEmployeeSession(t, h, m.org.ID, agent.ID, sb.ID, "slack", "Tool trace", now.Add(-time.Hour))
	seedEmployeeSessionEvent(t, h, session, "user.message.received", `{"text":"What happened?"}`, now.Add(-30*time.Minute), 1)
	seedEmployeeSessionEvent(t, h, session, "agent.tool.call", `{"tool":"railway","args":{"service":"api"}}`, now.Add(-20*time.Minute), 2)
	seedEmployeeSessionEvent(t, h, session, "agent.message.sent", `{"text":"Railway deploy succeeded."}`, now.Add(-10*time.Minute), 3)

	rr := h.listEmployeeSessionEvents(t, m, agent.ID, session.ID, "?limit=2")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var first struct {
		Data []struct {
			EventType      string          `json:"event_type"`
			SequenceNumber int64           `json:"sequence_number"`
			Payload        json.RawMessage `json:"payload"`
		} `json:"data"`
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if !first.HasMore || first.NextCursor == nil || len(first.Data) != 2 {
		t.Fatalf("first page = %#v", first)
	}
	if first.Data[0].EventType != "agent.message.sent" || first.Data[1].EventType != "agent.tool.call" {
		t.Fatalf("first page order = %#v", first.Data)
	}

	rr = h.listEmployeeSessionEvents(t, m, agent.ID, session.ID, "?limit=2&cursor="+*first.NextCursor)
	if rr.Code != http.StatusOK {
		t.Fatalf("second status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var second struct {
		Data []struct {
			EventType string          `json:"event_type"`
			Payload   json.RawMessage `json:"payload"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if second.HasMore || len(second.Data) != 1 || second.Data[0].EventType != "user.message.received" {
		t.Fatalf("second page = %#v", second)
	}
	if !json.Valid(second.Data[0].Payload) {
		t.Fatalf("payload is invalid json: %s", string(second.Data[0].Payload))
	}
}

func seedEmployeeSession(t *testing.T, h *employeeHarness, orgID, agentID, sandboxID uuid.UUID, source, name string, createdAt time.Time) model.EmployeeSession {
	t.Helper()
	session := model.EmployeeSession{
		OrgID:                 orgID,
		EmployeeID:            agentID,
		SandboxID:             sandboxID,
		RuntimeConversationID: "runtime-" + uuid.NewString(),
		Source:                source,
		SourceResourceKey:     "C123:" + createdAt.Format("150405"),
		Status:                "active",
		Name:                  name,
		IntegrationScopes:     model.JSON{},
		CreatedAt:             createdAt,
		UpdatedAt:             createdAt,
	}
	if err := h.db.Create(&session).Error; err != nil {
		t.Fatalf("create employee session: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", session.ID).Delete(&model.EmployeeSession{}) })
	return session
}

func seedEmployeeSessionEvent(t *testing.T, h *employeeHarness, session model.EmployeeSession, eventType, payload string, eventAt time.Time, sequence int64) model.EmployeeSessionEvent {
	t.Helper()
	event := model.EmployeeSessionEvent{
		OrgID:             session.OrgID,
		EmployeeID:        session.EmployeeID,
		SandboxID:         session.SandboxID,
		EmployeeSessionID: session.ID,
		SessionID:         session.RuntimeConversationID,
		EventID:           "evt-" + uuid.NewString(),
		EventType:         eventType,
		Source:            session.Source,
		Mode:              "employee",
		SequenceNumber:    sequence,
		Payload:           model.RawJSON(payload),
		EventAt:           eventAt,
		CreatedAt:         eventAt,
	}
	if err := h.db.Create(&event).Error; err != nil {
		t.Fatalf("create employee session event: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", event.ID).Delete(&model.EmployeeSessionEvent{}) })
	return event
}
