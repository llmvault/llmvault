package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsCreate_QueuesFirstMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	out := h.createSession(t, fx, fx.owner, "Investigate the deploy failure")
	if out.Session.ID == "" || out.Session.ChannelID != fx.channel.ID.String() {
		t.Fatalf("bad session response: %+v", out.Session)
	}
	if out.Session.AgentID != fx.agent.ID.String() || out.Session.ParticipantCount != 1 {
		t.Fatalf("bad agent/participant response: %+v", out.Session)
	}
	if !out.Queued || out.Event == nil || out.Event.SequenceNumber != 1 {
		t.Fatalf("bad queued event response: %+v", out)
	}
	if out.Event.Payload["text"] != "Investigate the deploy failure" {
		t.Fatalf("event payload=%#v", out.Event.Payload)
	}

	var queueCount int64
	if err := h.db.Model(&model.SessionMessageQueue{}).
		Where("session_id = ? AND status = ?", out.Session.ID, "pending").
		Count(&queueCount).Error; err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queueCount != 1 {
		t.Fatalf("queue count=%d, want 1", queueCount)
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
		"text":              "I can reproduce it",
		"user_display_name": "Member User",
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if !out.Queued || out.Event == nil || out.Event.SequenceNumber != 3 {
		t.Fatalf("message event=%+v queued=%v", out.Event, out.Queued)
	}

	var events []model.SessionEvent
	if err := h.db.Where("session_id = ?", created.Session.ID).
		Order("sequence_number ASC").
		Find(&events).Error; err != nil {
		t.Fatalf("load events: %v", err)
	}
	if got := eventTypes(events); len(got) != 3 || got[0] != "user.message" || got[1] != "participant.joined" || got[2] != "user.message" {
		t.Fatalf("event types=%v", got)
	}
	var queueRows []model.SessionMessageQueue
	if err := h.db.Where("session_id = ?", created.Session.ID).
		Order("sequence_number ASC").
		Find(&queueRows).Error; err != nil {
		t.Fatalf("load queue rows: %v", err)
	}
	if len(queueRows) != 2 || queueRows[0].SequenceNumber != 1 || queueRows[1].SequenceNumber != 3 {
		t.Fatalf("queue rows=%+v", queueRows)
	}
}

func TestIntegration_SessionsChannelVisibilityDoesNotGrantSend(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Shared channel visibility")
	if err := h.db.Create(&model.ChannelMember{ChannelID: fx.channel.ID, UserID: fx.viewer.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add viewer to channel: %v", err)
	}

	list := h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions", fx, fx.viewer, nil)
	assertSessionListIDs(t, list, []string{created.Session.ID})

	send := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.viewer, map[string]any{
		"text": "not a participant",
	})
	if send.Code != http.StatusForbidden {
		t.Fatalf("viewer send status=%d body=%s", send.Code, send.Body.String())
	}
}

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
