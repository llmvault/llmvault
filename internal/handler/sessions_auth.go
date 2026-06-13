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

func (h *SessionHandler) loadSession(ctx context.Context, orgID, sessionID uuid.UUID) (model.Session, bool, error) {
	var session model.Session
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", sessionID, orgID).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return session, false, nil
	}
	return session, err == nil, err
}

func (h *SessionHandler) authorizeSession(w http.ResponseWriter, r *http.Request, requireParticipant bool) (model.Session, *uuid.UUID, bool) {
	org, ok := sessionOrg(w, r)
	if !ok {
		return model.Session{}, nil, false
	}
	sessionID, ok := sessionIDFromRequest(w, r)
	if !ok {
		return model.Session{}, nil, false
	}
	session, found, err := h.loadSession(r.Context(), org.ID, sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session"})
		return model.Session{}, nil, false
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session not found"})
		return model.Session{}, nil, false
	}
	userID, _ := currentSessionUserID(r)
	if h.canAccessSession(r.Context(), session, userID, requireParticipant) {
		return session, userID, true
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Error: "session access denied"})
	return model.Session{}, nil, false
}

func (h *SessionHandler) canAccessSession(ctx context.Context, session model.Session, userID *uuid.UUID, requireParticipant bool) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	orgRole, err := h.orgRole(ctx, session.OrgID, userID)
	if err == nil && isOrgManager(orgRole) {
		return true
	}
	participant, _ := h.sessionParticipantRole(ctx, session.ID, userID)
	if requireParticipant {
		return participant != ""
	}
	if participant != "" {
		return true
	}
	channelRole, _ := h.channelMemberRole(ctx, session.ChannelID, userID)
	return channelRole != ""
}

func (h *SessionHandler) canUseChannel(ctx context.Context, channel model.Channel, userID *uuid.UUID) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	orgRole, err := h.orgRole(ctx, channel.OrgID, userID)
	if err == nil && isOrgManager(orgRole) {
		return true
	}
	role, _ := h.channelMemberRole(ctx, channel.ID, userID)
	return role != ""
}

func (h *SessionHandler) orgRole(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
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

func (h *SessionHandler) channelMemberRole(ctx context.Context, channelID uuid.UUID, userID *uuid.UUID) (string, error) {
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

func (h *SessionHandler) sessionParticipantRole(ctx context.Context, sessionID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var participant model.SessionParticipant
	err := h.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", sessionID, *userID).
		First(&participant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return participant.Role, err
}

func sessionOrg(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}
