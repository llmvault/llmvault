package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

const globalChannelName = "Global"

type memoryGroupResponse struct {
	ChannelID   *string          `json:"channel_id"`
	ChannelName string           `json:"channel_name"`
	Memories    []memoryResponse `json:"memories"`
	HasMore     bool             `json:"has_more"`
	Total       int              `json:"total"`
	NextCursor  *string          `json:"next_cursor,omitempty"`
}

type memoryGroupedResponse struct {
	Groups []memoryGroupResponse `json:"groups"`
}

// Grouped handles GET /v1/memories/grouped.
// @Summary List memories grouped by channel
// @Description Returns the newest memories for every channel in one call (default 10 per channel) with a per-channel has_more flag. Global (channel-less) memories are the first group. Use the per-channel endpoint to page past the initial slice.
// @Tags memories
// @Produce json
// @Param per_channel query int false "Max memories per channel (1-50, default 10)"
// @Success 200 {object} memoryGroupedResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/memories/grouped [get]
func (h *MemoryHandler) Grouped(w http.ResponseWriter, r *http.Request) {
	org, _, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	vis, err := listVisibility(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve channel visibility"})
		return
	}
	perChannel := parseMemoryLimit(r.URL.Query().Get("per_channel"), 10)
	groups, err := h.service().GroupedByChannel(r.Context(), org.ID, perChannel, vis)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	out := make([]memoryGroupResponse, len(groups))
	for i, group := range groups {
		memories := make([]memoryResponse, len(group.Memories))
		for j := range group.Memories {
			memories[j] = memoryToResponse(group.Memories[j], nil)
		}
		out[i] = memoryGroupResponse{
			ChannelID:   formatUUIDPtr(group.ChannelID),
			ChannelName: channelDisplayName(group),
			Memories:    memories,
			HasMore:     group.HasMore,
			Total:       group.Total,
		}
		if group.NextCursor != "" {
			cursor := group.NextCursor
			out[i].NextCursor = &cursor
		}
	}
	writeJSON(w, http.StatusOK, memoryGroupedResponse{Groups: out})
}

// ListChannel handles GET /v1/memories/channels/{channelId}.
// @Summary Page one channel's memories
// @Description Cursor-paginated memories for a single channel, newest first. Use channelId "global" for channel-less organization memories, or a channel UUID. Pass the previous response's next_cursor to fetch the next page.
// @Tags memories
// @Produce json
// @Param channelId path string true "Channel UUID or \"global\""
// @Param cursor query string false "Pagination cursor from a previous response"
// @Param limit query int false "Max items per page (1-50, default 10)"
// @Success 200 {object} paginatedResponse[memoryResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/memories/channels/{channelId} [get]
func (h *MemoryHandler) ListChannel(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	scope, ok := parseChannelParam(w, chi.URLParam(r, "channelId"))
	if !ok {
		return
	}
	// A by-channel read names a channel as a resource: a non-manager asking for a
	// channel they cannot use is denied (403), not handed an empty page. Global
	// (channel-less) reads are open to everyone in the org.
	if scope.ChannelID != nil && !h.canReadChannelMemories(r, org.ID, userID, *scope.ChannelID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "channel access denied"})
		return
	}
	limit := parseMemoryLimit(r.URL.Query().Get("limit"), 10)
	rows, next, hasMore, err := h.service().ListChannelPage(
		r.Context(), org.ID, scope, strings.TrimSpace(r.URL.Query().Get("cursor")), limit,
	)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	items := make([]memoryResponse, len(rows))
	for i := range rows {
		items[i] = memoryToResponse(rows[i], nil)
	}
	resp := paginatedResponse[memoryResponse]{Data: items, HasMore: hasMore}
	if next != "" {
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseChannelParam turns the {channelId} path segment into a channel scope:
// "global" reads channel-less memories, a UUID reads that channel only.
func parseChannelParam(w http.ResponseWriter, raw string) (memory.ChannelScope, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "global" || raw == "org" {
		return memory.ChannelScope{}, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channelId must be a UUID or \"global\""})
		return memory.ChannelScope{}, false
	}
	return memory.ChannelScope{ChannelID: &id}, true
}

// canReadChannelMemories reports whether the actor may read one channel's
// memories. Managers and API-key callers always may; everyone else must be able
// to use the channel under the team-based visibility predicate. A channel that
// does not exist (or is archived) is treated as not readable so the endpoint
// never confirms an unusable channel's existence to a non-member.
func (h *MemoryHandler) canReadChannelMemories(r *http.Request, orgID uuid.UUID, userID *uuid.UUID, channelID uuid.UUID) bool {
	ctx := r.Context()
	apiKey := isAPIKeyRequest(ctx)
	role, err := orgRoleForUser(ctx, h.db, orgID, userID)
	if err != nil {
		return false
	}
	if apiKey || isOrgManager(role) {
		return true
	}
	var channel model.Channel
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		First(&channel).Error; err != nil {
		return false
	}
	return canUseChannel(ctx, h.db, channel, role, userID, apiKey)
}

func channelDisplayName(group memory.GroupedChannel) string {
	if group.ChannelID == nil || strings.TrimSpace(group.ChannelName) == "" {
		return globalChannelName
	}
	return group.ChannelName
}
