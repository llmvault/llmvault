package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestIntegration_SessionsUpdateRejectsChannelMoveWhenAgentUnavailable verifies
// the team-primary move guard: a session may not be moved into a team-scoped
// channel whose team excludes the session's agent (channelagents.ActsInChannel).
// This is the enforcement that replaced the cut agent_channels allowlist — and
// it closes the former hole where session channel-move checked only the legacy
// allowlist.
func TestIntegration_SessionsUpdateRejectsChannelMoveWhenAgentUnavailable(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	// A team the session's (team-less) agent does not belong to, plus a channel
	// owned by that team. Moving the session there must be rejected.
	team := model.Team{OrgID: fx.org.ID, Name: "walled-" + uuid.NewString()[:8]}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	teamChannel := seedSessionChannel(t, h, fx, "walled", fx.agent.ID, &team.ID)
	created := h.createSession(t, fx, fx.owner, "Initial team-less session")

	move := h.doJSON(t, http.MethodPatch, "/v1/sessions/"+created.Session.ID, fx, fx.owner, map[string]any{
		"channel_id": teamChannel.ID.String(),
	})
	if move.Code != http.StatusForbidden {
		t.Fatalf("move into foreign-team channel status=%d body=%s", move.Code, move.Body.String())
	}
}

func seedSessionChannel(t *testing.T, h *sessionHarness, fx sessionFixture, name string, agentID uuid.UUID, teamID *uuid.UUID) model.Channel {
	t.Helper()
	channel := model.Channel{
		OrgID:          fx.org.ID,
		Name:           name + "-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		TeamID:         teamID,
		DefaultAgentID: agentID,
		Origin:         "native",
		CreatedBy:      &fx.owner.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create session channel: %v", err)
	}
	return channel
}
