package tasks

import (
	"context"

	"github.com/usehivy/hivy/internal/agentrouter"
	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// routeChannelAgent picks which assigned agent handles an inbound message on an
// existing channel. It runs the deterministic + LLM routing layers over the
// channel's assigned agents. Thread affinity is resolved by the caller before
// this runs and always wins; this is only reached when a message opens (or
// continues without a prior session mapping) a thread in a known channel.
//
// It never returns an error path that would block the message: any lookup
// failure resolves to the channel default agent.
func (h *SlackAppMentionHandler) routeChannelAgent(ctx context.Context, row *model.SlackThreadEvent, channel model.Channel) (model.Agent, error) {
	assigned, err := channelagents.ListAssigned(ctx, h.db, channel.OrgID, channel.ID)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "slack_route_list_assigned_failed",
			"channel_id", channel.ID.String(), "error", err)
		return h.loadAgent(ctx, channel.OrgID, channel.DefaultAgentID)
	}

	// Zero or one assigned agent: nothing to route between. The single-agent
	// short-circuit here also avoids listing recent sessions and any LLM call.
	if len(assigned) <= 1 {
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
		Candidates:     agentrouter.CandidatesFromAgents(assigned),
		RecentSessions: recent,
		DefaultAgentID: channel.DefaultAgentID,
	})

	return h.loadAgent(ctx, channel.OrgID, decision.AgentID)
}
