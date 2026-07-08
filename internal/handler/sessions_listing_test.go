package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIntegration_ChannelMembersSeeAllSessionsButSendNeedsParticipant(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Shared channel visibility")

	// A channel member sees every session in the channel, even ones they didn't create.
	list := h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions", fx, fx.bystander, nil)
	assertSessionListIDs(t, list, []string{created.Session.ID})

	send := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.bystander, map[string]any{
		"text": "not a participant",
	})
	if send.Code != http.StatusForbidden {
		t.Fatalf("bystander send status=%d body=%s", send.Code, send.Body.String())
	}
}

func TestIntegration_ChannelSessionsIncludeAllSourcesForMembers(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	base := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	memberSession := seedActivitySession(t, h, fx, fx.member.ID, nil, base.Add(time.Hour))
	slackSession := seedSlackActivitySession(t, h, fx, base.Add(2*time.Hour))

	// A member sees every session in the channel regardless of source or creator.
	assertSessionListIDs(t, h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions?sort=activity", fx, fx.bystander, nil), []string{slackSession.ID.String(), memberSession.ID.String()})

	detail := h.doJSON(t, http.MethodGet, "/v1/sessions/"+slackSession.ID.String(), fx, fx.bystander, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("slack session detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	events := h.doJSON(t, http.MethodGet, "/v1/sessions/"+slackSession.ID.String()+"/events", fx, fx.bystander, nil)
	if events.Code != http.StatusOK {
		t.Fatalf("slack session events status=%d body=%s", events.Code, events.Body.String())
	}
	send := h.doJSON(t, http.MethodPost, "/v1/sessions/"+slackSession.ID.String()+"/messages", fx, fx.bystander, map[string]any{
		"text": "web reply",
	})
	if send.Code != http.StatusConflict {
		t.Fatalf("slack session send status=%d body=%s", send.Code, send.Body.String())
	}
}

func TestIntegration_ChannelSessionsActivitySortShowsAllChannelSessions(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	base := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	oldest := seedActivitySession(t, h, fx, fx.owner.ID, nil, base)
	newest := seedActivitySession(t, h, fx, fx.member.ID, []uuid.UUID{fx.member.ID}, base.Add(2*time.Hour))
	middle := seedActivitySession(t, h, fx, fx.member.ID, []uuid.UUID{fx.owner.ID}, base.Add(time.Hour))

	// A channel member sees every session in the channel, newest activity first,
	// regardless of who created it or whether they participate.
	want := []uuid.UUID{newest.ID, middle.ID, oldest.ID}
	cursor := ""
	for i, wantID := range want {
		path := "/v1/channels/" + fx.channel.ID.String() + "/sessions?sort=activity&limit=1"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		rr := h.doJSON(t, http.MethodGet, path, fx, fx.owner, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("page %d status=%d body=%s", i, rr.Code, rr.Body.String())
		}
		var page struct {
			Data       []sessionOut `json:"data"`
			HasMore    bool         `json:"has_more"`
			NextCursor *string      `json:"next_cursor"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode page %d: %v\n%s", i, err, rr.Body.String())
		}
		if len(page.Data) != 1 || page.Data[0].ID != wantID.String() {
			t.Fatalf("page %d sessions=%+v, want %s", i, page.Data, wantID)
		}
		if last := i == len(want)-1; last {
			if page.HasMore || page.NextCursor != nil {
				t.Fatalf("page %d should be the last: %+v", i, page)
			}
		} else {
			if !page.HasMore || page.NextCursor == nil {
				t.Fatalf("page %d missing continuation: %+v", i, page)
			}
			cursor = *page.NextCursor
		}
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
