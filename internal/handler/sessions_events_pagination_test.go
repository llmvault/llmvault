package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestSessionEventsPaginationUsesSequenceWhenBatchTimestampsMatch(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "event-sequence-pagination")
	agent := seedTeamAgent(t, db, org.ID, team.ID)
	session := model.Session{
		ID:              uuid.New(),
		OrgID:           org.ID,
		TeamID:          team.ID,
		AgentID:         agent.ID,
		Status:          "active",
		AgentTurnStatus: model.SessionAgentTurnIdle,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("session_id = ?", session.ID).Delete(&model.SessionEvent{})
		db.Where("id = ?", session.ID).Delete(&model.Session{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	batchTime := time.Now().UTC().Truncate(time.Second)
	events := make([]model.SessionEvent, 150)
	for index := range events {
		sequence := int64(index + 1)
		eventType := "token"
		if sequence == 150 {
			eventType = "turn_completed"
		}
		events[index] = model.SessionEvent{
			ID:             uuid.New(),
			OrgID:          org.ID,
			SessionID:      session.ID,
			AgentID:        agent.ID,
			EventID:        "event-" + strconv.FormatInt(sequence, 10),
			EventType:      eventType,
			Source:         "session",
			SequenceNumber: sequence,
			Payload:        model.JSON{"sequence": sequence},
			EventAt:        batchTime,
			CreatedAt:      batchTime,
		}
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create session event batch: %v", err)
	}

	sessionHandler := handler.NewSessionHandler(db)
	router := chi.NewRouter()
	router.Get("/v1/sessions/{id}/events", sessionHandler.ListEvents)
	requestPage := func(cursor string) eventPage {
		t.Helper()
		path := "/v1/sessions/" + session.ID.String() + "/events?limit=100"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		orgCopy := org
		request = middleware.WithOrg(request, &orgCopy)
		request = middleware.WithAPIKeyClaims(
			request,
			&middleware.APIKeyClaims{OrgID: org.ID.String(), Scopes: []string{"sessions"}},
		)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("list events status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var page eventPage
		if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode events page: %v", err)
		}
		return page
	}

	first := requestPage("")
	if len(first.Data) != 100 || first.Data[0].SequenceNumber != 150 || first.Data[99].SequenceNumber != 51 {
		t.Fatalf("first page sequence range = %d..%d with %d events, want 150..51 with 100", first.Data[0].SequenceNumber, first.Data[len(first.Data)-1].SequenceNumber, len(first.Data))
	}
	if first.Data[0].EventType != "turn_completed" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page terminal event or cursor missing: %+v", first)
	}
	second := requestPage(first.NextCursor)
	if len(second.Data) != 50 || second.Data[0].SequenceNumber != 50 || second.Data[49].SequenceNumber != 1 || second.HasMore {
		t.Fatalf("second page sequence range = %d..%d with %d events, want 50..1 with 50", second.Data[0].SequenceNumber, second.Data[len(second.Data)-1].SequenceNumber, len(second.Data))
	}
}

func TestSessionTranscriptEventsCompactRuntimeDeltasBeforePagination(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	team := seedTeam(t, db, org.ID, "event-transcript-compaction")
	agent := seedTeamAgent(t, db, org.ID, team.ID)
	session := model.Session{
		ID:              uuid.New(),
		OrgID:           org.ID,
		TeamID:          team.ID,
		AgentID:         agent.ID,
		Status:          "active",
		AgentTurnStatus: model.SessionAgentTurnIdle,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		db.Where("session_id = ?", session.ID).Delete(&model.SessionEvent{})
		db.Where("id = ?", session.ID).Delete(&model.Session{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
	})

	batchTime := time.Now().UTC().Truncate(time.Second)
	turnID := "turn-transcript"
	events := []model.SessionEvent{
		transcriptFixtureEvent(org.ID, session.ID, agent.ID, 1, "user.message.received", "", batchTime),
		transcriptFixtureEvent(org.ID, session.ID, agent.ID, 2, "turn_started", turnID, batchTime),
	}
	sequence := int64(3)
	for range 150 {
		thinking := transcriptFixtureEvent(org.ID, session.ID, agent.ID, sequence, "thinking", turnID, batchTime)
		thinking.Payload["text"] = "r"
		events = append(events, thinking)
		sequence++
		events = append(events, transcriptFixtureEvent(org.ID, session.ID, agent.ID, sequence, "model_usage", turnID, batchTime))
		sequence++
		token := transcriptFixtureEvent(org.ID, session.ID, agent.ID, sequence, "token", turnID, batchTime)
		token.Payload["text"] = "a"
		events = append(events, token)
		sequence++
	}
	events = append(events, transcriptFixtureEvent(org.ID, session.ID, agent.ID, sequence, "tool_result", turnID, batchTime))
	sequence++
	final := transcriptFixtureEvent(org.ID, session.ID, agent.ID, sequence, "final", turnID, batchTime)
	final.Payload["text"] = "answer"
	events = append(events, final)
	sequence++
	events = append(events, transcriptFixtureEvent(org.ID, session.ID, agent.ID, sequence, "turn_completed", turnID, batchTime))
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create transcript event batch: %v", err)
	}

	sessionHandler := handler.NewSessionHandler(db)
	router := chi.NewRouter()
	router.Get("/v1/sessions/{id}/events", sessionHandler.ListEvents)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+session.ID.String()+"/events?limit=100&view=transcript", nil)
	orgCopy := org
	request = middleware.WithOrg(request, &orgCopy)
	request = middleware.WithAPIKeyClaims(
		request,
		&middleware.APIKeyClaims{OrgID: org.ID.String(), Scopes: []string{"sessions"}},
	)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list transcript events status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var page eventPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode transcript events page: %v", err)
	}
	if page.HasMore || page.NextCursor != "" {
		t.Fatalf("compacted transcript unexpectedly has another page: %+v", page)
	}
	if len(page.Data) != 6 {
		t.Fatalf("compacted transcript event count = %d, want 6", len(page.Data))
	}
	counts := make(map[string]int)
	var thinkingPayload map[string]any
	for _, event := range page.Data {
		counts[event.EventType]++
		if event.EventType == "thinking" {
			thinkingPayload = event.Payload
		}
	}
	if counts["model_usage"] != 0 || counts["token"] != 0 || counts["thinking"] != 1 {
		t.Fatalf("unexpected compacted event types: %+v", counts)
	}
	if text, _ := thinkingPayload["text"].(string); len(text) != 150 {
		t.Fatalf("coalesced thinking text length = %d, want 150", len(text))
	}
	if deltaCount, _ := thinkingPayload["delta_count"].(float64); deltaCount != 150 {
		t.Fatalf("coalesced thinking delta count = %v, want 150", thinkingPayload["delta_count"])
	}
}

func transcriptFixtureEvent(orgID, sessionID, agentID uuid.UUID, sequence int64, eventType, turnID string, at time.Time) model.SessionEvent {
	return model.SessionEvent{
		ID:             uuid.New(),
		OrgID:          orgID,
		SessionID:      sessionID,
		AgentID:        agentID,
		EventID:        "event-" + strconv.FormatInt(sequence, 10),
		EventType:      eventType,
		Source:         "runtime",
		SequenceNumber: sequence,
		TurnID:         turnID,
		Payload:        model.JSON{"sequence": sequence},
		EventAt:        at,
		CreatedAt:      at,
	}
}

type eventPage struct {
	Data []struct {
		EventType      string         `json:"event_type"`
		SequenceNumber int64          `json:"sequence_number"`
		Payload        map[string]any `json:"payload"`
	} `json:"data"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}
