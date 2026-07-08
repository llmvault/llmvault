package channelagents

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// VisibleAgentIDsSubquery yields the ids of agents assigned to channels the
// given user is allowed to use, per the team-based channel visibility
// predicate. It is the read-side analogue of Assigned: where Assigned answers
// "may this agent run in this channel", this answers "which agents may a
// non-manager user see at all", so the /v1/agents surface and the list_agents
// MCP tool cannot drift from the channel-visibility rule.
//
// The predicate mirrors handler.visibleTeamSubquery exactly (do not fork a new
// membership definition): a channel is visible when it is externally sourced,
// has no team scope (team_id IS NULL), or is scoped to a team the user actively
// belongs to (via a non-archived team). Managers and API-key callers bypass
// this entirely at the call site and see org-wide.
//
// System-kind channels are excluded. System channels hold trigger/schedule
// sessions and are exempt from assignment (see Assigned) — they normally carry
// no channel_agents rows, so the join alone would already exclude them, but the
// explicit kind guard guarantees a stray runtime-created row can never leak
// extra agent visibility to a non-member.
//
// A nil userID mirrors visibleTeamSubquery's empty-membership case: only
// external and team_id-NULL channels' agents remain visible. The returned value
// is a subquery for use in `agents.id IN (?)`; pass the base *gorm.DB (not a
// context-bound query) as db, matching handler.visibleTeamSubquery's callers.
func VisibleAgentIDsSubquery(db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) *gorm.DB {
	return db.Model(&model.ChannelAgent{}).
		Select("channel_agents.agent_id").
		Joins("JOIN channels ON channels.id = channel_agents.channel_id").
		Where("channel_agents.org_id = ?", orgID).
		Where("channels.kind <> ?", "system").
		Where("(channels.visibility <> ? AND (channels.origin = ? OR channels.team_id IS NULL OR channels.team_id IN (?))) OR channels.id IN (?)",
			"private", "external", visibleTeamSubquery(db, userID), memberChannelIDSubquery(db, userID))
}

// VisibleChannelIDsSubquery yields the ids of channels the given user is allowed
// to use, per the team-based channel visibility predicate. It is the channel_id
// analogue of VisibleAgentIDsSubquery: memory/observation/directive list reads
// filter on `channel_id IS NULL OR channel_id IN (this)` so a non-manager never
// receives rows referencing channels they cannot use.
//
// The predicate mirrors handler.canUseChannel / visibleTeamSubquery exactly (do
// not fork a new membership definition): a channel is usable when it is
// externally sourced, has no team scope (team_id IS NULL), or is scoped to a
// team the user actively belongs to (via a non-archived team). Managers and
// API-key callers bypass this entirely at the call site and see org-wide.
//
// A nil userID mirrors visibleTeamSubquery's empty-membership case: only
// external and team_id-NULL channels remain visible. The returned value is a
// subquery for use in `channel_id IN (?)`; pass the base *gorm.DB (not a
// context-bound query) as db, matching VisibleAgentIDsSubquery's callers.
func VisibleChannelIDsSubquery(db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) *gorm.DB {
	return db.Model(&model.Channel{}).
		Select("channels.id").
		Where("channels.org_id = ?", orgID).
		Where("(channels.visibility <> ? AND (channels.origin = ? OR channels.team_id IS NULL OR channels.team_id IN (?))) OR channels.id IN (?)",
			"private", "external", visibleTeamSubquery(db, userID), memberChannelIDSubquery(db, userID))
}

// visibleTeamSubquery is an exact copy of handler.visibleTeamSubquery. It lives
// here because the handler helper is unexported and channelagents must not
// depend on the handler package; the two must stay in lockstep.
func visibleTeamSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.TeamMember{}).
		Select("team_members.team_id").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("team_members.user_id = ?", *userID)
}

// memberChannelIDSubquery yields the ids of channels the given user is an
// explicit member of, re-admitting private channels excluded by the team-scoped
// predicate above. Mirror of handler.memberChannelIDSubquery.
func memberChannelIDSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.ChannelMember{}).Select("channel_id")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("user_id = ?", *userID)
}
