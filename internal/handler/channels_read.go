package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// List handles GET /v1/channels.
// @Summary List channels
// @Description Lists channels visible to the caller. Use discoverable=true to include public channels the caller can join.
// @Tags channels
// @Produce json
// @Param discoverable query bool false "Include public discoverable channels"
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
		if r.URL.Query().Get("discoverable") == "true" {
			q = q.Where("visibility = ? OR id IN (?)", "public", h.memberChannelSubquery(userID))
		} else {
			q = q.Where("id IN (?)", h.memberChannelSubquery(userID))
		}
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
	result := h.channelListResponses(ctx, channels, userID)
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
	writeJSON(w, http.StatusOK, channelDetailResponse{
		Channel: channelToResponse(channel, role, h.memberCount(ctx, channel.ID)),
		Members: h.channelMembers(ctx, channel.ID),
	})
}

func (h *ChannelHandler) memberChannelSubquery(userID *uuid.UUID) *gorm.DB {
	q := h.db.Model(&model.ChannelMember{}).Select("channel_id")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("user_id = ?", *userID)
}

func (h *ChannelHandler) channelListResponses(ctx context.Context, channels []model.Channel, userID *uuid.UUID) []channelResponse {
	ids := make([]uuid.UUID, len(channels))
	for i, channel := range channels {
		ids[i] = channel.ID
	}
	counts := h.memberCounts(ctx, ids)
	roles := h.channelRoles(ctx, ids, userID)
	out := make([]channelResponse, len(channels))
	for i, channel := range channels {
		out[i] = channelToResponse(channel, roles[channel.ID], counts[channel.ID])
	}
	return out
}
