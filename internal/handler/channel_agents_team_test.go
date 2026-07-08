package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// seedTeamScopedAgent inserts an active agent owned by the given team.
func seedTeamScopedAgent(t *testing.T, h *channelHarness, orgID, teamID uuid.UUID) model.Agent {
	t.Helper()
	agent := model.Agent{
		OrgID:         &orgID,
		TeamID:        &teamID,
		Name:          "team-agent-" + uuid.NewString()[:8],
		Model:         "deepseek-v4-flash",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create team agent: %v", err)
	}
	return agent
}

// A team-scoped agent may not be assigned to a channel of a different team.
func TestIntegration_ChannelAgentsRejectCrossTeam(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	teamA := model.Team{OrgID: fx.org.ID, Name: "ca-teamA-" + uuid.NewString()[:8]}
	teamB := model.Team{OrgID: fx.org.ID, Name: "ca-teamB-" + uuid.NewString()[:8]}
	if err := h.db.Create(&teamA).Error; err != nil {
		t.Fatalf("create teamA: %v", err)
	}
	if err := h.db.Create(&teamB).Error; err != nil {
		t.Fatalf("create teamB: %v", err)
	}
	// Channel is scoped to team A.
	channel := seedChannelAgentsChannel(t, h, fx, &teamA.ID)
	base := "/v1/channels/" + channel.ID.String() + "/agents"

	// An agent owned by team B is cross-team -> 422.
	agentB := seedTeamScopedAgent(t, h, fx.org.ID, teamB.ID)
	cross := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"agent_id": agentB.ID.String()})
	if cross.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cross-team assign status=%d, want 422; body=%s", cross.Code, cross.Body.String())
	}

	// An agent owned by team A is same-team -> 201.
	agentA := seedTeamScopedAgent(t, h, fx.org.ID, teamA.ID)
	same := h.doJSON(t, http.MethodPost, base, fx, fx.owner, map[string]any{"agent_id": agentA.ID.String()})
	if same.Code != http.StatusCreated {
		t.Fatalf("same-team assign status=%d, want 201; body=%s", same.Code, same.Body.String())
	}
}

// A no-team (or external) channel accepts any agent: the cross-team rule only
// applies when both sides carry a team.
func TestIntegration_ChannelAgentsNoTeamChannelAcceptsTeamAgent(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	team := model.Team{OrgID: fx.org.ID, Name: "ca-team-" + uuid.NewString()[:8]}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	channel := seedChannelAgentsChannel(t, h, fx, nil) // no-team channel
	agent := seedTeamScopedAgent(t, h, fx.org.ID, team.ID)
	rr := h.doJSON(t, http.MethodPost, "/v1/channels/"+channel.ID.String()+"/agents", fx, fx.owner,
		map[string]any{"agent_id": agent.ID.String()})
	if rr.Code != http.StatusCreated {
		t.Fatalf("no-team channel assign status=%d, want 201; body=%s", rr.Code, rr.Body.String())
	}
}
