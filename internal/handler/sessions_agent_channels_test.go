package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsCreateRespectsAgentChannelRestrictions(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	other := seedSessionChannel(t, h, fx, "support", fx.agent.ID, nil)
	if err := h.db.Create(&model.AgentChannel{OrgID: fx.org.ID, AgentID: fx.agent.ID, ChannelID: fx.channel.ID}).Error; err != nil {
		t.Fatalf("restrict agent to channel: %v", err)
	}

	disallowed := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": other.ID.String(),
		"text":       "Try restricted default agent",
	})
	if disallowed.Code != http.StatusForbidden {
		t.Fatalf("disallowed create status=%d body=%s", disallowed.Code, disallowed.Body.String())
	}

	allowed := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Use restricted agent in allowed channel",
	})
	if allowed.Code != http.StatusCreated {
		t.Fatalf("allowed create status=%d body=%s", allowed.Code, allowed.Body.String())
	}

	unrestricted := seedSessionAgent(t, h.db, fx.org.ID)
	// The target channel is team-less, so any org agent may act in it under the
	// team-primary model — the explicit unrestricted agent needs no assignment.
	explicit := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": other.ID.String(),
		"agent_id":   unrestricted.ID.String(),
		"text":       "Use unrestricted explicit agent",
	})
	if explicit.Code != http.StatusCreated {
		t.Fatalf("explicit unrestricted create status=%d body=%s", explicit.Code, explicit.Body.String())
	}
}

func TestIntegration_SessionsUpdateRejectsChannelMoveWhenAgentUnavailable(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	other := seedSessionChannel(t, h, fx, "support", fx.agent.ID, nil)
	created := h.createSession(t, fx, fx.owner, "Initial allowed session")
	if err := h.db.Create(&model.AgentChannel{OrgID: fx.org.ID, AgentID: fx.agent.ID, ChannelID: fx.channel.ID}).Error; err != nil {
		t.Fatalf("restrict agent to channel: %v", err)
	}

	move := h.doJSON(t, http.MethodPatch, "/v1/sessions/"+created.Session.ID, fx, fx.owner, map[string]any{
		"channel_id": other.ID.String(),
	})
	if move.Code != http.StatusForbidden {
		t.Fatalf("move status=%d body=%s", move.Code, move.Body.String())
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
