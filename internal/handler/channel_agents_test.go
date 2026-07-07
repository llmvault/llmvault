package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// channelAgentsListOut mirrors the {"data": [<agent>]} response.
type channelAgentsListOut struct {
	Data []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"data"`
}

func seedChannelAgentsChannel(t *testing.T, h *channelHarness, fx channelFixture, teamID *uuid.UUID) model.Channel {
	t.Helper()
	channel := model.Channel{
		OrgID:          fx.org.ID,
		Name:           "ca-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		TeamID:         teamID,
		DefaultAgentID: fx.agent.ID,
		Origin:         "native",
		CreatedBy:      &fx.owner.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := h.db.Create(&model.ChannelAgent{OrgID: fx.org.ID, ChannelID: channel.ID, AgentID: fx.agent.ID}).Error; err != nil {
		t.Fatalf("assign default agent: %v", err)
	}
	return channel
}

func seedChannelAgentsAgent(t *testing.T, h *channelHarness, orgID uuid.UUID, status string) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:         &orgID,
		Name:          "ca-agent-" + uuid.NewString()[:8],
		Model:         "deepseek-v4-flash",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        status,
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func TestIntegration_ChannelAgentsAssignListUnassign(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channel := seedChannelAgentsChannel(t, h, fx, nil)
	second := seedChannelAgentsAgent(t, h, fx.org.ID, "active")

	base := "/v1/channels/" + channel.ID.String() + "/agents"

	// Assign the second agent.
	rr := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"agent_id": second.ID.String()})
	if rr.Code != http.StatusCreated {
		t.Fatalf("assign status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Duplicate assign -> 409.
	dup := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"agent_id": second.ID.String()})
	if dup.Code != http.StatusConflict {
		t.Fatalf("dup status=%d body=%s", dup.Code, dup.Body.String())
	}
	// List shows both (default + second).
	list := h.doJSON(t, http.MethodGet, base, fx, fx.owner, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var out channelAgentsListOut
	if err := json.Unmarshal(list.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, list.Body.String())
	}
	if len(out.Data) != 2 {
		t.Fatalf("list len=%d, want 2: %s", len(out.Data), list.Body.String())
	}

	// Unassign the non-default second agent -> 204.
	del := h.doJSON(t, http.MethodDelete, base+"/"+second.ID.String(), fx, fx.owner, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("unassign status=%d body=%s", del.Code, del.Body.String())
	}
	// Unassigning the default agent -> 422.
	delDefault := h.doJSON(t, http.MethodDelete, base+"/"+fx.agent.ID.String(), fx, fx.owner, nil)
	if delDefault.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unassign default status=%d body=%s", delDefault.Code, delDefault.Body.String())
	}
}

func TestIntegration_ChannelAgentsAssignArchivedIs422(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channel := seedChannelAgentsChannel(t, h, fx, nil)
	archived := seedChannelAgentsAgent(t, h, fx.org.ID, "archived")

	rr := h.doJSON(t, http.MethodPost, "/v1/channels/"+channel.ID.String()+"/agents", fx, fx.owner,
		map[string]any{"agent_id": archived.ID.String()})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("assign archived status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_ChannelAgentsMembershipGated403(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	// A team-scoped channel: the org "member" (not on the team) is not a member.
	team := model.Team{OrgID: fx.org.ID, Name: "team-" + uuid.NewString()[:8]}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	channel := seedChannelAgentsChannel(t, h, fx, &team.ID)
	second := seedChannelAgentsAgent(t, h, fx.org.ID, "active")
	base := "/v1/channels/" + channel.ID.String() + "/agents"

	// Non-member POST -> 403.
	post := h.doJSON(t, http.MethodPost, base, fx, fx.member, map[string]any{"agent_id": second.ID.String()})
	if post.Code != http.StatusForbidden {
		t.Fatalf("non-member assign status=%d body=%s", post.Code, post.Body.String())
	}
	// Non-member DELETE -> 403.
	del := h.doJSON(t, http.MethodDelete, base+"/"+second.ID.String(), fx, fx.member, nil)
	if del.Code != http.StatusForbidden {
		t.Fatalf("non-member unassign status=%d body=%s", del.Code, del.Body.String())
	}

	// A team member can manage.
	if err := h.db.Create(&model.TeamMember{OrgID: fx.org.ID, TeamID: team.ID, UserID: fx.member.ID}).Error; err != nil {
		t.Fatalf("add team member: %v", err)
	}
	ok := h.doJSON(t, http.MethodPost, base, fx, fx.member, map[string]any{"agent_id": second.ID.String()})
	if ok.Code != http.StatusCreated {
		t.Fatalf("member assign status=%d body=%s", ok.Code, ok.Body.String())
	}
}

func TestIntegration_ChannelDefaultChangeAutoAssigns(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channel := seedChannelAgentsChannel(t, h, fx, nil)
	next := seedChannelAgentsAgent(t, h, fx.org.ID, "active")

	// Changing the default to an unassigned agent auto-assigns it.
	rr := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channel.ID.String(), fx, fx.owner,
		map[string]any{"default_agent_id": next.ID.String()})
	if rr.Code != http.StatusOK {
		t.Fatalf("update default status=%d body=%s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := h.db.Model(&model.ChannelAgent{}).
		Where("channel_id = ? AND agent_id = ?", channel.ID, next.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("new default not auto-assigned: rows=%d", count)
	}
}
