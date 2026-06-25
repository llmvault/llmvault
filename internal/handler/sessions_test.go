package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

func TestIntegration_SessionsCreate_QueuesFirstMessage(t *testing.T) {
	contextBuilder := &recordingPreContextBuilder{sections: []string{"## Relevant memories\n- Queued initial context"}}
	h := newSessionHarnessWith(t, func(hdl *handler.SessionHandler) {
		hdl.WithPreContextBuilder(contextBuilder)
	})
	fx := h.seed(t)

	out := h.createSession(t, fx, fx.owner, "Investigate the deploy failure")
	if out.Session.ID == "" || out.Session.ChannelID != fx.channel.ID.String() {
		t.Fatalf("bad session response: %+v", out.Session)
	}
	if out.Session.AgentID != fx.agent.ID.String() || out.Session.ParticipantCount != 1 {
		t.Fatalf("bad agent/participant response: %+v", out.Session)
	}
	if !out.Queued || out.Event == nil {
		t.Fatalf("bad queued command response: %+v", out)
	}
	if out.Event.EventType != runtimeevents.EventUserMessageReceived || out.Event.Source != "web" || out.Event.Payload["text"] != "Investigate the deploy failure" {
		t.Fatalf("bad backend event: %+v", out.Event)
	}
	if _, ok := out.Event.Payload["dynamic_context"]; ok {
		t.Fatalf("backend event should not store dynamic_context: %+v", out.Event.Payload)
	}

	var queueRows []model.SessionMessageQueue
	if err := h.db.Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status = ?", out.Session.ID, "pending").
		Order("sequence_number ASC").
		Find(&queueRows).Error; err != nil {
		t.Fatalf("load queue: %v", err)
	}
	if len(queueRows) != 1 || queueRows[0].SequenceNumber != 1 || queueRows[0].MessageText != "Investigate the deploy failure" {
		t.Fatalf("queue rows=%+v, want one pending command", queueRows)
	}
	if queueRows[0].SessionEventID == nil || queueRows[0].SessionEventID.String() != out.Event.ID {
		t.Fatalf("queue session_event_id=%v, want response event %s", queueRows[0].SessionEventID, out.Event.ID)
	}
	if got := contextBuilder.calls; got != 1 {
		t.Fatalf("precontext calls=%d, want 1", got)
	}
	if got, ok := queueRows[0].MessagePayload["_session_context"].([]any); !ok || len(got) != 1 || got[0] != "## Relevant memories\n- Queued initial context" {
		t.Fatalf("queue session_context=%#v", queueRows[0].MessagePayload["_session_context"])
	}

	assertSessionNameTaskEnqueued(t, h, out.Session.ID)
}

func TestIntegration_SessionsCreate_AllowsEmptySessionBeforeFirstMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create empty session status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := decodeSessionMutation(t, rr)
	if out.Session.ID == "" || out.Session.ParticipantCount != 1 {
		t.Fatalf("bad empty session response: %+v", out.Session)
	}
	if out.Queued || out.Event != nil {
		t.Fatalf("empty session queued/event=%v/%+v, want no message dispatch", out.Queued, out.Event)
	}
	var queueCount int64
	if err := h.db.Model(&model.SessionMessageQueue{}).Where("session_id = ?", out.Session.ID).Count(&queueCount).Error; err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queueCount != 0 {
		t.Fatalf("queue count=%d, want 0", queueCount)
	}
}

func TestIntegration_SessionsCreate_RejectsLegacyPayloadFields(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id":      fx.channel.ID.String(),
		"text":            "Start clean",
		"client_event_id": "old-client-id",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_SessionsGet_ExposesAgentTurnState(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Track this turn")
	startedAt := time.Now().UTC()
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", created.Session.ID).
		Updates(map[string]any{
			"agent_turn_status":     model.SessionAgentTurnActive,
			"agent_turn_id":         "turn-123",
			"agent_stream_id":       "stream-123",
			"agent_turn_started_at": startedAt,
		}).Error; err != nil {
		t.Fatalf("mark active turn: %v", err)
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID, fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get session status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Session sessionOut `json:"session"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode session: %v\n%s", err, rr.Body.String())
	}
	if out.Session.AgentTurnStatus != model.SessionAgentTurnActive ||
		out.Session.AgentTurnID != "turn-123" ||
		out.Session.AgentStreamID != "stream-123" ||
		out.Session.AgentTurnStartedAt == nil {
		t.Fatalf("missing turn metadata: %+v", out.Session)
	}
}

func TestIntegration_SessionsRespondToInput_QueuesAnswerAndRecordsMarker(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Ask me later")

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/input-responses", fx, fx.owner, map[string]any{
		"request_id": "question-1",
		"text":       "Use option A",
	})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("input response status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := decodeSessionMutation(t, rr)
	if !out.Queued || out.Event == nil {
		t.Fatalf("bad input response command: %+v", out)
	}
	if out.Event.EventType != runtimeevents.EventUserMessageReceived || out.Event.Payload["text"] != "Use option A" {
		t.Fatalf("bad input response event: %+v", out.Event)
	}
	if _, ok := out.Event.Payload["client_event_id"]; ok {
		t.Fatalf("input response event still includes client_event_id: %+v", out.Event.Payload)
	}
	var queueRows []model.SessionMessageQueue
	if err := h.db.
		Where("session_id = ?", created.Session.ID).
		Order("sequence_number ASC").
		Find(&queueRows).Error; err != nil {
		t.Fatalf("load queue: %v", err)
	}
	if len(queueRows) != 2 || queueRows[1].MessageText != "Use option A" {
		t.Fatalf("queue rows=%+v, want input response as second command", queueRows)
	}
	if queueRows[1].SessionEventID == nil || queueRows[1].SessionEventID.String() != out.Event.ID {
		t.Fatalf("input queue session_event_id=%v, want response event %s", queueRows[1].SessionEventID, out.Event.ID)
	}
	input, ok := queueRows[1].MessagePayload["input_response"].(map[string]any)
	if !ok || input["request_id"] != "question-1" || input["text"] != "Use option A" {
		t.Fatalf("input response payload=%#v", queueRows[1].MessagePayload["input_response"])
	}
}

func TestIntegration_SessionsCreate_AllowsCatalogModelWhenInstalledModelsAreStale(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	catalog := model.AgentCatalog{
		Name:            "Catalog-" + fx.agent.ID.String(),
		Slug:            "catalog-" + fx.agent.ID.String(),
		Model:           "deepseek-v4-pro",
		AvailableModels: []string{"deepseek-v4-pro", "qwen3.7-plus"},
		SubAgents:       model.RawJSON("{}"),
		Manifest:        model.RawJSON("{}"),
		Status:          model.AgentCatalogStatusActive,
	}
	if err := h.db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })
	if err := h.db.Model(&model.Agent{}).
		Where("id = ?", fx.agent.ID).
		Updates(map[string]any{
			"agent_catalog_id": catalog.ID,
			"model":            "qwen3.7-plus",
			"available_models": pq.StringArray{"deepseek-v4-flash"},
		}).Error; err != nil {
		t.Fatalf("update agent with stale models: %v", err)
	}

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Start with the catalog model",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := decodeSessionMutation(t, rr)
	var stored model.Session
	if err := h.db.First(&stored, "id = ?", out.Session.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if stored.Model != "qwen3.7-plus" {
		t.Fatalf("session model = %q, want qwen3.7-plus", stored.Model)
	}
}

func TestIntegration_SessionsParticipantsAndQueuedMessages(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Kick off the incident room")

	invite := h.doJSON(t, http.MethodPut, "/v1/sessions/"+created.Session.ID+"/participants/"+fx.member.ID.String(), fx, fx.owner, nil)
	if invite.Code != http.StatusOK {
		t.Fatalf("invite status=%d body=%s", invite.Code, invite.Body.String())
	}

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.member, map[string]any{
		"text": "I can reproduce it",
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if !out.Queued || out.Event == nil {
		t.Fatalf("message event=%+v queued=%v", out.Event, out.Queued)
	}
	if out.Event.EventType != runtimeevents.EventUserMessageReceived || out.Event.Payload["text"] != "I can reproduce it" {
		t.Fatalf("bad participant event: %+v", out.Event)
	}
	var queueRows []model.SessionMessageQueue
	if err := h.db.Where("session_id = ?", created.Session.ID).
		Order("sequence_number ASC").
		Find(&queueRows).Error; err != nil {
		t.Fatalf("load queue rows: %v", err)
	}
	if len(queueRows) != 2 || queueRows[0].SequenceNumber != 1 || queueRows[1].SequenceNumber != 2 {
		t.Fatalf("queue rows=%+v", queueRows)
	}
	if queueRows[1].MessageText != "I can reproduce it" || queueRows[1].ActorUserID == nil || *queueRows[1].ActorUserID != fx.member.ID {
		t.Fatalf("queued participant command=%+v", queueRows[1])
	}
	if queueRows[1].SessionEventID == nil || queueRows[1].SessionEventID.String() != out.Event.ID {
		t.Fatalf("participant queue session_event_id=%v, want response event %s", queueRows[1].SessionEventID, out.Event.ID)
	}
}

func TestIntegration_SessionsSend_RejectsLegacyClientEventID(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create empty session status=%d body=%s", created.Code, created.Body.String())
	}
	session := decodeSessionMutation(t, created)
	body := map[string]any{
		"text":            "Retry this only once",
		"client_event_id": "client-msg-1",
	}
	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+session.Session.ID+"/messages", fx, fx.owner, body)
	if msg.Code != http.StatusBadRequest {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
}
