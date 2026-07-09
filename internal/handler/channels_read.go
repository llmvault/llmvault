package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const defaultChannelRecentSessionsLimit = 5
const maxChannelRecentSessionsLimit = 20

// List handles GET /v1/channels.
// @Summary List channels
// @Description Lists channels visible to the caller. Use discoverable=true to include public channels the caller can join.
// @Tags channels
// @Produce json
// @Param discoverable query bool false "Include public discoverable channels"
// @Param include query string false "Comma-separated optional expansions. Supports recent_sessions."
// @Param recent_sessions_limit query int false "Recent sessions per channel when include=recent_sessions (default 5, max 20)"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[channelResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels [get]
func (h *ChannelHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := orgForChannelRequest(w, ctx)
	if !ok {
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	includeRecentSessions := includeChannelRecentSessions(r)
	recentSessionsLimit, err := parseChannelRecentSessionsLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	userID, _ := currentRequestUserID(ctx)
	orgRole, err := h.currentUserOrgRole(ctx, org.ID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load org membership"})
		return
	}

	q := h.db.WithContext(ctx).
		Model(&model.Channel{}).
		Where("org_id = ? AND archived_at IS NULL", org.ID)
	if !isAPIKeyRequest(ctx) && !isOrgManager(orgRole) {
		q = q.Where(
			"(channels.visibility <> ? AND channels.team_id IN (?)) OR channels.id IN (?)",
			"private", visibleTeamSubquery(h.db, userID), memberChannelIDSubquery(h.db, userID))
	}
	q = applyPagination(q, cursor, limit)

	var channels []model.Channel
	if err := q.Find(&channels).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list channels"})
		return
	}
	hasMore := len(channels) > limit
	if hasMore {
		channels = channels[:limit]
	}
	result := h.channelListResponses(ctx, channels, userID, includeRecentSessions, recentSessionsLimit)
	resp := paginatedResponse[channelResponse]{Data: result, HasMore: hasMore}
	if hasMore {
		last := channels[len(channels)-1]
		cursor := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /v1/channels/{id}.
// @Summary Get a channel
// @Description Returns one visible channel and its members.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id} [get]
func (h *ChannelHandler) Get(w http.ResponseWriter, r *http.Request) {
	channel, _, role, ok := h.authorizeChannel(w, r, false)
	if !ok {
		return
	}
	ctx := r.Context()
	resp := channelToResponse(channel, role, h.memberCount(ctx, channel.ID))
	resp.TeamName = h.teamNamesForChannels(ctx, []model.Channel{channel})[channel.TeamID]
	writeJSON(w, http.StatusOK, channelDetailResponse{
		Channel: resp,
		Members: h.channelMembers(ctx, channel.ID),
	})
}

// canCreateChannelInTeam reports whether the caller may create a channel bound
// to teamID. Reuses the channel-management predicate probed against the would-be
// channel's team: API keys and org managers always may; a non-manager must be an
// active member of the target team.
func (h *ChannelHandler) canCreateChannelInTeam(ctx context.Context, orgID uuid.UUID, teamID uuid.UUID) bool {
	userID, _ := currentRequestUserID(ctx)
	orgRole, _ := h.currentUserOrgRole(ctx, orgID, userID)
	return canManageChannel(ctx, h.db, model.Channel{OrgID: orgID, TeamID: teamID}, userID, orgRole, isAPIKeyRequest(ctx))
}

// teamNamesForChannels resolves display names for the (unarchived) teams that
// own the given channels, keyed by team id. One query regardless of channel
// count.
func (h *ChannelHandler) teamNamesForChannels(ctx context.Context, channels []model.Channel) map[uuid.UUID]string {
	ids := make([]uuid.UUID, 0, len(channels))
	seen := map[uuid.UUID]struct{}{}
	for _, c := range channels {
		if _, dup := seen[c.TeamID]; dup {
			continue
		}
		seen[c.TeamID] = struct{}{}
		ids = append(ids, c.TeamID)
	}
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out
	}
	var teams []model.Team
	if err := h.db.WithContext(ctx).
		Select("id", "name").
		Where("id IN ? AND archived_at IS NULL", ids).
		Find(&teams).Error; err != nil {
		return out
	}
	for _, t := range teams {
		out[t.ID] = t.Name
	}
	return out
}

func includeChannelRecentSessions(r *http.Request) bool {
	for _, value := range strings.Split(r.URL.Query().Get("include"), ",") {
		if strings.TrimSpace(value) == "recent_sessions" {
			return true
		}
	}
	return false
}

func parseChannelRecentSessionsLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("recent_sessions_limit"))
	if raw == "" {
		return defaultChannelRecentSessionsLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, errInvalidRecentSessionsLimit()
	}
	if limit > maxChannelRecentSessionsLimit {
		limit = maxChannelRecentSessionsLimit
	}
	return limit, nil
}

func errInvalidRecentSessionsLimit() error {
	return &channelListParamError{message: "recent_sessions_limit must be a positive integer"}
}

type channelListParamError struct {
	message string
}

func (e *channelListParamError) Error() string {
	return e.message
}

func (h *ChannelHandler) channelListResponses(ctx context.Context, channels []model.Channel, userID *uuid.UUID, includeRecentSessions bool, recentSessionsLimit int) []channelResponse {
	ids := make([]uuid.UUID, len(channels))
	for i, channel := range channels {
		ids[i] = channel.ID
	}
	counts := h.memberCounts(ctx, ids)
	roles := h.channelRoles(ctx, ids, userID)
	teamNames := h.teamNamesForChannels(ctx, channels)
	recent := map[uuid.UUID]channelRecentSessions{}
	if includeRecentSessions {
		recent = h.recentSessionsByChannel(ctx, ids, userID, recentSessionsLimit)
	}
	out := make([]channelResponse, len(channels))
	for i, channel := range channels {
		out[i] = channelToResponse(channel, roles[channel.ID], counts[channel.ID])
		out[i].TeamName = teamNames[channel.TeamID]
		if includeRecentSessions {
			recentForChannel := recent[channel.ID]
			out[i].RecentSessions = &recentForChannel.Sessions
			out[i].RecentSessionsHasMore = recentForChannel.HasMore
			out[i].RecentSessionsNextCursor = recentForChannel.NextCursor
		}
	}
	return out
}

type channelRecentSessions struct {
	Sessions   []sessionResponse
	HasMore    bool
	NextCursor *string
}

func (h *ChannelHandler) recentSessionsByChannel(ctx context.Context, channelIDs []uuid.UUID, userID *uuid.UUID, limit int) map[uuid.UUID]channelRecentSessions {
	out := make(map[uuid.UUID]channelRecentSessions, len(channelIDs))
	for _, id := range channelIDs {
		out[id] = channelRecentSessions{Sessions: []sessionResponse{}}
	}
	if len(channelIDs) == 0 || userID == nil || limit < 1 {
		return out
	}

	var sessions []model.Session
	query := `
WITH ranked_sessions AS (
  SELECT
    s.*,
    ROW_NUMBER() OVER (
      PARTITION BY s.channel_id
      ORDER BY COALESCE(session_activity.last_event, s.updated_at) DESC, s.created_at DESC, s.id DESC
    ) AS recent_rank
  FROM sessions s
  LEFT JOIN LATERAL (
    SELECT max(event_at) AS last_event
    FROM session_events se
    WHERE se.session_id = s.id
  ) AS session_activity ON TRUE
  WHERE s.channel_id IN ?
    AND s.status <> ?
)
SELECT *
FROM ranked_sessions
WHERE recent_rank <= ?
ORDER BY channel_id ASC, recent_rank ASC`
	if err := h.db.WithContext(ctx).Raw(query, channelIDs, "archived", limit+1).Scan(&sessions).Error; err != nil {
		return out
	}

	grouped := make(map[uuid.UUID][]model.Session, len(channelIDs))
	for _, session := range sessions {
		grouped[session.ChannelID] = append(grouped[session.ChannelID], session)
	}
	for channelID, channelSessions := range grouped {
		hasMore := len(channelSessions) > limit
		if hasMore {
			channelSessions = channelSessions[:limit]
		}
		stats := statsForSessionsDB(ctx, h.db, sessionIDsV2(channelSessions))
		result := channelRecentSessions{
			Sessions: sessionResponsesFromStats(channelSessions, stats),
			HasMore:  hasMore,
		}
		if hasMore && len(channelSessions) > 0 {
			last := channelSessions[len(channelSessions)-1]
			next := encodeActivityCursor(sessionActivityTime(last, stats[last.ID]), last.CreatedAt, last.ID)
			result.NextCursor = &next
		}
		out[channelID] = result
	}
	return out
}
