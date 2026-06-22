package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

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

func TestIntegration_SessionsCreate_WithExplicitName_DoesNotEnqueueAutoNaming(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Investigate the deploy failure",
		"name":       "manual-name",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}

	for _, task := range h.enqueuer.Tasks() {
		if task.TypeName == tasks.TypeSessionName {
			t.Fatalf("unexpected %s task enqueued", tasks.TypeSessionName)
		}
	}
}
