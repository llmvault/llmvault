package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestPrivateChannelVisibility asserts that channels.visibility == "private"
// gates a channel to its explicit members even when its origin/team would
// otherwise open it to the whole org: a private external channel and a private
// team-less channel must be invisible/unusable to a non-member org member, yet
// visible/usable to a channel member and to org managers.
func TestPrivateChannelVisibility(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	admin := h.seedUser(t, fx.org.ID, "admin")
	outsider := h.seedUser(t, fx.org.ID, "member")
	insider := h.seedUser(t, fx.org.ID, "member")

	// A private channel that is EITHER externally sourced OR team-less — both are
	// normally open to any org member, so both must be gated by private.
	extCh := model.Channel{
		OrgID: fx.org.ID, Name: "priv-ext-" + uuid.NewString()[:8], Kind: "standard",
		Origin: "external", Visibility: "private", DefaultAgentID: fx.agent.ID,
	}
	nullCh := model.Channel{
		OrgID: fx.org.ID, Name: "priv-null-" + uuid.NewString()[:8], Kind: "standard",
		Visibility: "private", DefaultAgentID: fx.agent.ID,
	}
	for _, c := range []*model.Channel{&extCh, &nullCh} {
		if err := h.db.Create(c).Error; err != nil {
			t.Fatalf("create channel: %v", err)
		}
		if err := h.db.Create(&model.ChannelMember{ChannelID: c.ID, UserID: insider.ID, Role: "member"}).Error; err != nil {
			t.Fatalf("add channel member: %v", err)
		}
	}
	t.Cleanup(func() {
		h.db.Where("channel_id IN ?", []uuid.UUID{extCh.ID, nullCh.ID}).Delete(&model.ChannelMember{})
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

	// A non-member org member must neither list nor view either private channel.
	out := listed(outsider)
	if out[extCh.ID.String()] || out[nullCh.ID.String()] {
		t.Fatalf("outsider must not list private channels: %v", out)
	}
	if code := getStatus(outsider, extCh.ID); code != http.StatusForbidden {
		t.Fatalf("outsider get external = %d, want 403", code)
	}
	if code := getStatus(outsider, nullCh.ID); code != http.StatusForbidden {
		t.Fatalf("outsider get team-less = %d, want 403", code)
	}

	// A channel member sees and can view them.
	in := listed(insider)
	if !in[extCh.ID.String()] || !in[nullCh.ID.String()] {
		t.Fatalf("insider must list both private channels: %v", in)
	}
	if code := getStatus(insider, extCh.ID); code != http.StatusOK {
		t.Fatalf("insider get external = %d, want 200", code)
	}

	// Org managers keep org-wide visibility.
	adm := listed(admin)
	if !adm[extCh.ID.String()] || !adm[nullCh.ID.String()] {
		t.Fatalf("admin must list both private channels: %v", adm)
	}
	if code := getStatus(admin, nullCh.ID); code != http.StatusOK {
		t.Fatalf("admin get team-less = %d, want 200", code)
	}
}

// TestResolveDefaultAgentID_SameTeam asserts that a team-scoped channel's default
// agent must belong to the channel's team: picking a cross-team agent as the
// default is rejected (422, closing the channel_agents same-team bypass), while a
// same-team default is accepted and auto-assigned to the channel.
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
	mkAgent := func(team *uuid.UUID) model.Agent {
		a := model.Agent{OrgID: &fx.org.ID, Name: "a-" + uuid.NewString()[:8], Model: "test", Status: "active", TeamID: team}
		if err := h.db.Create(&a).Error; err != nil {
			t.Fatalf("create agent: %v", err)
		}
		return a
	}
	agentX := mkAgent(&teamX.ID)
	agentY := mkAgent(&teamY.ID)

	// teamX channel with a teamY default agent -> rejected.
	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name": "cross-" + uuid.NewString()[:8], "category": "general", "team_id": teamX.ID.String(), "default_agent_id": agentY.ID.String(),
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-team default = %d, want 422; body=%s", rr.Code, rr.Body.String())
	}

	// teamX channel with a teamX default agent -> accepted, and the default agent
	// is assigned to the channel (channel_agents row).
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
	var count int64
	if err := h.db.Model(&model.ChannelAgent{}).
		Where("channel_id = ? AND agent_id = ?", chID, agentX.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count channel agents: %v", err)
	}
	if count != 1 {
		t.Fatalf("default agent not assigned to channel: channel_agents count = %d, want 1", count)
	}
}
