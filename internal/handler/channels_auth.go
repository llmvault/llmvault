package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *ChannelHandler) loadChannel(ctx context.Context, orgID, channelID uuid.UUID) (model.Channel, bool, error) {
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return channel, false, nil
	}
	return channel, err == nil, err
}

func (h *ChannelHandler) currentUserOrgRole(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var membership model.OrgMembership
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, *userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return membership.Role, err
}

func (h *ChannelHandler) channelRole(ctx context.Context, channelID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var member model.ChannelMember
	err := h.db.WithContext(ctx).
		Where("channel_id = ? AND user_id = ?", channelID, *userID).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return member.Role, err
}

func isOrgManager(role string) bool {
	return role == "owner" || role == "admin"
}

func canViewChannel(channel model.Channel, orgRole, memberRole string, apiKey bool) bool {
	if apiKey || isOrgManager(orgRole) || memberRole != "" {
		return true
	}
	return channel.Visibility == "public"
}

func canManageChannel(orgRole, memberRole string, apiKey bool) bool {
	return apiKey || isOrgManager(orgRole) || memberRole == "owner"
}

func (h *ChannelHandler) authorizeChannel(w http.ResponseWriter, r *http.Request, requireManage bool) (model.Channel, *uuid.UUID, string, bool) {
	ctx := r.Context()
	org, ok := orgForChannelRequest(w, ctx)
	if !ok {
		return model.Channel{}, nil, "", false
	}
	channelID, ok := channelIDFromRequest(w, r)
	if !ok {
		return model.Channel{}, nil, "", false
	}
	channel, found, err := h.loadChannel(ctx, org.ID, channelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channel"})
		return model.Channel{}, nil, "", false
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "channel not found"})
		return model.Channel{}, nil, "", false
	}
	userID, _ := currentRequestUserID(ctx)
	orgRole, err := h.currentUserOrgRole(ctx, org.ID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load org membership"})
		return model.Channel{}, nil, "", false
	}
	memberRole, err := h.channelRole(ctx, channel.ID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channel membership"})
		return model.Channel{}, nil, "", false
	}
	apiKey := isAPIKeyRequest(ctx)
	allowed := canViewChannel(channel, orgRole, memberRole, apiKey)
	if requireManage {
		allowed = canManageChannel(orgRole, memberRole, apiKey)
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "channel access denied"})
		return model.Channel{}, nil, "", false
	}
	return channel, userID, memberRole, true
}

func orgForChannelRequest(w http.ResponseWriter, ctx context.Context) (*model.Org, bool) {
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}
