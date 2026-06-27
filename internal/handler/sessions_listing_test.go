package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntegration_SessionsChannelVisibilityDoesNotGrantReadOrSend(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Shared channel visibility")

	list := h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions", fx, fx.viewer, nil)
	assertSessionListIDs(t, list, nil)

	send := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.viewer, map[string]any{
		"text": "not a participant",
	})
	if send.Code != http.StatusForbidden {
		t.Fatalf("viewer send status=%d body=%s", send.Code, send.Body.String())
	}
}

func TestIntegration_ChannelSessionsIncludeSlackSourceWithoutParticipant(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	base := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	_ = seedActivitySession(t, h, fx, fx.member.ID, nil, base.Add(time.Hour))
	slackSession := seedSlackActivitySession(t, h, fx, base.Add(2*time.Hour))

	assertSessionListIDs(t, h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions?sort=activity", fx, fx.viewer, nil), []string{slackSession.ID.String()})

	detail := h.doJSON(t, http.MethodGet, "/v1/sessions/"+slackSession.ID.String(), fx, fx.viewer, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("slack session detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	events := h.doJSON(t, http.MethodGet, "/v1/sessions/"+slackSession.ID.String()+"/events", fx, fx.viewer, nil)
	if events.Code != http.StatusOK {
		t.Fatalf("slack session events status=%d body=%s", events.Code, events.Body.String())
	}
	send := h.doJSON(t, http.MethodPost, "/v1/sessions/"+slackSession.ID.String()+"/messages", fx, fx.viewer, map[string]any{
		"text": "web reply",
	})
	if send.Code != http.StatusConflict {
		t.Fatalf("slack session send status=%d body=%s", send.Code, send.Body.String())
	}
}

func TestIntegration_ChannelSessionsActivitySortIsScopedToCurrentUser(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	base := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	visibleOld := seedActivitySession(t, h, fx, fx.owner.ID, nil, base)
	hidden := seedActivitySession(t, h, fx, fx.member.ID, []uuid.UUID{fx.member.ID}, base.Add(2*time.Hour))
	visibleNew := seedActivitySession(t, h, fx, fx.member.ID, []uuid.UUID{fx.owner.ID}, base.Add(time.Hour))

	first := h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions?sort=activity&limit=1", fx, fx.owner, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", first.Code, first.Body.String())
	}
	var firstPage struct {
		Data       []sessionOut `json:"data"`
		HasMore    bool         `json:"has_more"`
		NextCursor *string      `json:"next_cursor"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("decode first page: %v\n%s", err, first.Body.String())
	}
	if len(firstPage.Data) != 1 || firstPage.Data[0].ID != visibleNew.ID.String() {
		t.Fatalf("first page sessions=%+v, want %s", firstPage.Data, visibleNew.ID)
	}
	if !firstPage.HasMore || firstPage.NextCursor == nil {
		t.Fatalf("first page missing continuation: %+v", firstPage)
	}

	second := h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions?sort=activity&limit=1&cursor="+url.QueryEscape(*firstPage.NextCursor), fx, fx.owner, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", second.Code, second.Body.String())
	}
	var secondPage struct {
		Data []sessionOut `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("decode second page: %v\n%s", err, second.Body.String())
	}
	if len(secondPage.Data) != 1 || secondPage.Data[0].ID != visibleOld.ID.String() {
		t.Fatalf("second page sessions=%+v, want %s", secondPage.Data, visibleOld.ID)
	}
	if hidden.ID == uuid.Nil {
		t.Fatal("hidden session was not seeded")
	}
}

func TestIntegration_SessionsCreate_RejectsExplicitName(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Investigate the deploy failure",
		"name":       "manual-name",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
}
