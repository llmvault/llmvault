package tasks

import (
	"context"

	"github.com/usehivy/hivy/internal/agentrouter"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// routeChannelAgent picks which of the channel's team agents handles an inbound
// message on an existing channel. Under the team-primary model the candidate
// pool is every active top-level agent belonging to the channel's owning team
// (agents.team_id == channel.team_id). It runs the deterministic + LLM routing
// layers over that pool. Thread affinity is resolved by the caller before this
// runs and always wins; this is only reached when a message opens (or continues
// without a prior session mapping) a thread in a known channel.
//
// It never returns an error path that would block the message: any lookup
// failure resolves to the channel default agent. External channels are
// team-scoped like native ones and route over their team pool; a team with zero
// or one agent falls back to the channel default agent below.
func (h *SlackAppMentionHandler) routeChannelAgent(ctx context.Context, row *model.SlackThreadEvent, channel model.Channel) (model.Agent, error) {
	var candidates []model.Agent
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND parent_agent_id IS NULL AND status <> ?",
			channel.OrgID, channel.TeamID, "archived").
		Order("is_default DESC, name ASC").
		Find(&candidates).Error; err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "slack_route_list_team_agents_failed",
			"channel_id", channel.ID.String(), "error", err)
		return h.loadAgent(ctx, channel.OrgID, channel.DefaultAgentID)
	}

	// Zero or one candidate: nothing to route between. The single-agent
	// short-circuit here also avoids listing recent sessions and any LLM call.
	if len(candidates) <= 1 {
		return h.loadAgent(ctx, channel.OrgID, channel.DefaultAgentID)
	}

	recent, err := agentrouter.RecentChannelSessions(ctx, h.db, channel.OrgID, channel.ID)
	if err != nil {
		// Continuity context is best-effort; route without it.
		logging.FromContext(ctx).WarnContext(ctx, "slack_route_recent_sessions_failed",
			"channel_id", channel.ID.String(), "error", err)
		recent = nil
	}

	router := agentrouter.New(h.db, h.cacheManager)
	decision := router.Route(ctx, agentrouter.Input{
		MessageText:    row.Text,
		Candidates:     agentrouter.CandidatesFromAgents(candidates),
		RecentSessions: recent,
		DefaultAgentID: channel.DefaultAgentID,
	})

	return h.loadAgent(ctx, channel.OrgID, decision.AgentID)
}
