package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// Join handles POST /v1/channels/{id}/join.
// @Summary Join a public channel
// @Description Adds the authenticated user to a public channel.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/join [post]
func (h *ChannelHandler) Join(w http.ResponseWriter, r *http.Request) {
	channel, userID, _, ok := h.authorizeChannel(w, r, false)
	if !ok {
		return
	}
	if userID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "user context is required"})
		return
	}
	if channel.Visibility != "public" {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "private channels require an invite"})
		return
	}
	member := model.ChannelMember{ChannelID: channel.ID, UserID: *userID, Role: "member"}
	if err := h.db.WithContext(r.Context()).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&member).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to join channel"})
		return
	}
	writeJSON(w, http.StatusOK, channelMutationResponse{
		Channel: channelToResponse(channel, "member", h.memberCount(r.Context(), channel.ID)),
	})
}

// PutMember handles PUT /v1/channels/{id}/members/{userID}.
// @Summary Add or update a channel member
// @Description Adds a user to a channel or updates their channel role.
// @Tags channels
// @Accept json
// @Produce json
// @Param id path string true "Channel ID"
// @Param userID path string true "User ID"
// @Param body body channelMemberRequest false "Member role"
// @Success 200 {object} channelDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/members/{userID} [put]
func (h *ChannelHandler) PutMember(w http.ResponseWriter, r *http.Request) {
	channel, _, role, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}
	targetUserID, ok := channelUserIDFromRequest(w, r)
	if !ok {
		return
	}
	req := channelMemberRequest{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	memberRole := defaultString(cleanStringPtr(req.Role), "member")
	if !validChannelMemberRole(memberRole) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "role must be owner or member"})
		return
	}
	if !h.userBelongsToOrg(r.Context(), channel.OrgID, targetUserID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "user must belong to this org"})
		return
	}
	member := model.ChannelMember{ChannelID: channel.ID, UserID: targetUserID, Role: memberRole}
	if err := h.db.WithContext(r.Context()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"role"}),
	}).Create(&member).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update channel member"})
		return
	}
	writeJSON(w, http.StatusOK, channelDetailResponse{
		Channel: channelToResponse(channel, role, h.memberCount(r.Context(), channel.ID)),
		Members: h.channelMembers(r.Context(), channel.ID),
	})
}

// DeleteMember handles DELETE /v1/channels/{id}/members/{userID}.
// @Summary Remove a channel member
// @Description Removes a user from a channel. Users may remove themselves; owners and org admins may remove others.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Param userID path string true "User ID"
// @Success 200 {object} channelDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/members/{userID} [delete]
func (h *ChannelHandler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	channel, userID, role, ok := h.authorizeChannel(w, r, false)
	if !ok {
		return
	}
	targetUserID, ok := channelUserIDFromRequest(w, r)
	if !ok {
		return
	}
	if !h.canRemoveChannelMember(r, channel, userID, targetUserID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "channel access denied"})
		return
	}
	if h.removingLastOwner(r, channel.ID, targetUserID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "cannot remove the last channel owner"})
		return
	}
	if err := h.db.WithContext(r.Context()).
		Where("channel_id = ? AND user_id = ?", channel.ID, targetUserID).
		Delete(&model.ChannelMember{}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to remove channel member"})
		return
	}
	writeJSON(w, http.StatusOK, channelDetailResponse{
		Channel: channelToResponse(channel, role, h.memberCount(r.Context(), channel.ID)),
		Members: h.channelMembers(r.Context(), channel.ID),
	})
}

func validChannelMemberRole(role string) bool {
	return role == "owner" || role == "member"
}

func (h *ChannelHandler) userBelongsToOrg(ctx context.Context, orgID, userID uuid.UUID) bool {
	var count int64
	_ = h.db.WithContext(ctx).
		Model(&model.OrgMembership{}).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Count(&count).Error
	return count == 1
}

// canRemoveChannelMember reports whether the caller may remove `target` from the
// channel. Anyone may remove themselves; removing another member is a manage-the-
// channel action under the team-primary model (canManageChannel: API keys and
// org managers, else an active member of the channel's owning team). The old
// channel-member "owner" role no longer grants this, so no member-role param.
func (h *ChannelHandler) canRemoveChannelMember(r *http.Request, channel model.Channel, userID *uuid.UUID, target uuid.UUID) bool {
	if userID != nil && *userID == target {
		return true
	}
	orgRole, _ := h.currentUserOrgRole(r.Context(), channel.OrgID, userID)
	return canManageChannel(r.Context(), h.db, channel, userID, orgRole, isAPIKeyRequest(r.Context()))
}

func (h *ChannelHandler) removingLastOwner(r *http.Request, channelID, userID uuid.UUID) bool {
	var target model.ChannelMember
	if err := h.db.WithContext(r.Context()).
		Where("channel_id = ? AND user_id = ?", channelID, userID).
		First(&target).Error; err != nil || target.Role != "owner" {
		return false
	}
	var owners int64
	_ = h.db.WithContext(r.Context()).
		Model(&model.ChannelMember{}).
		Where("channel_id = ? AND role = ?", channelID, "owner").
		Count(&owners).Error
	return owners <= 1
}
