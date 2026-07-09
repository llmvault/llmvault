package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestPrivateChannelVisibility asserts that channels.visibility == "private"
// gates a channel to its explicit members even when its team would otherwise
// open it to the whole team: a private channel must be invisible/unusable to a
// non-member org member, yet visible/usable to a channel member and to org
// managers.
func TestPrivateChannelVisibility(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	admin := h.seedUser(t, fx.org.ID, "admin")
	outsider := h.seedUser(t, fx.org.ID, "member")
	insider := h.seedUser(t, fx.org.ID, "member")

	// A private channel scoped to the fixture team — its team membership would
	// normally open it to team members, so private must gate it to its members.
	extCh := model.Channel{
		OrgID: fx.org.ID, Name: "priv-ext-" + uuid.NewString()[:8], Kind: "standard",
		Origin: "external", Visibility: "private", TeamID: fx.team.ID, DefaultAgentID: fx.agent.ID,
	}
	if err := h.db.Create(&extCh).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := h.db.Create(&model.ChannelMember{ChannelID: extCh.ID, UserID: insider.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add channel member: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("channel_id = ?", extCh.ID).Delete(&model.ChannelMember{})
	})

	listed := func(user model.User) map[string]bool {
		rr := h.doJSON(t, http.MethodGet, "/v1/channels", fx, user, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var out channelListOut
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		ids := map[string]bool{}
		for _, c := range out.Data {
			ids[c.ID] = true
		}
		return ids
	}
	getStatus := func(user model.User, chID uuid.UUID) int {
		rr := h.doJSON(t, http.MethodGet, "/v1/channels/"+chID.String(), fx, user, nil)
		return rr.Code
	}

	// A non-member org member must neither list nor view the private channel.
	out := listed(outsider)
	if out[extCh.ID.String()] {
		t.Fatalf("outsider must not list private channel: %v", out)
	}
	if code := getStatus(outsider, extCh.ID); code != http.StatusForbidden {
		t.Fatalf("outsider get private = %d, want 403", code)
	}

	// A channel member sees and can view it.
	in := listed(insider)
	if !in[extCh.ID.String()] {
		t.Fatalf("insider must list the private channel: %v", in)
	}
	if code := getStatus(insider, extCh.ID); code != http.StatusOK {
		t.Fatalf("insider get private = %d, want 200", code)
	}

	// Org managers keep org-wide visibility.
	adm := listed(admin)
	if !adm[extCh.ID.String()] {
		t.Fatalf("admin must list the private channel: %v", adm)
	}
	if code := getStatus(admin, extCh.ID); code != http.StatusOK {
		t.Fatalf("admin get private = %d, want 200", code)
	}
}

// TestResolveDefaultAgentID_SameTeam asserts that a team-scoped channel's default
// agent must belong to the channel's team: picking a cross-team agent as the
// default is rejected (422), while a same-team default is accepted and recorded
// as the channel's default_agent_id (the agent is usable by team ownership).
func TestResolveDefaultAgentID_SameTeam(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t) // fx.owner is an org owner (manager) so the create team gate passes

	teamX := model.Team{OrgID: fx.org.ID, Name: "tx-" + uuid.NewString()[:8]}
	teamY := model.Team{OrgID: fx.org.ID, Name: "ty-" + uuid.NewString()[:8]}
	if err := h.db.Create(&teamX).Error; err != nil {
		t.Fatalf("create teamX: %v", err)
	}
	if err := h.db.Create(&teamY).Error; err != nil {
		t.Fatalf("create teamY: %v", err)
	}
	mkAgent := func(team uuid.UUID) model.Agent {
		a := model.Agent{OrgID: &fx.org.ID, Name: "a-" + uuid.NewString()[:8], Model: "test", Status: "active", TeamID: team}
		if err := h.db.Create(&a).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
		return a
	}
	agentX := mkAgent(teamX.ID)
	agentY := mkAgent(teamY.ID)

	// teamX channel with a teamY default agent -> rejected.
	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name": "cross-" + uuid.NewString()[:8], "category": "general", "team_id": teamX.ID.String(), "default_agent_id": agentY.ID.String(),
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-team default = %d, want 422; body=%s", rr.Code, rr.Body.String())
	}

	// teamX channel with a teamX default agent -> accepted, and the channel's
	// default_agent_id records that agent (usable by virtue of team ownership).
	rr = h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name": "same-" + uuid.NewString()[:8], "category": "general", "team_id": teamX.ID.String(), "default_agent_id": agentX.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("same-team default = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	created := decodeChannelCreate(t, rr)
	chID, err := uuid.Parse(created.Channel.ID)
	if err != nil {
		t.Fatalf("parse channel id: %v", err)
	}
	var channel model.Channel
	if err := h.db.First(&channel, "id = ?", chID).Error; err != nil {
		t.Fatalf("load created channel: %v", err)
	}
	if channel.DefaultAgentID != agentX.ID {
		t.Fatalf("channel default_agent_id = %s, want same-team agent %s", channel.DefaultAgentID, agentX.ID)
	}
}
