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
	// Read access is private-by-default: a non-participant may
	// read/stream/enumerate a session only when they created it or manage its
	// channel. Bare channel-view no longer grants access, so one member can no
	// longer read another member's session in a public, team-less, or external
	// channel (PR-triggered transcripts included). Org managers already returned
	// above (oversight preserved); a manager of the channel's owning team is
	// admitted here.
	if userID != nil && session.CreatedBy != nil && *session.CreatedBy == *userID {
		return true
	}
	channel, found, err := h.loadSessionChannel(ctx, session)
	return err == nil && found && h.canManageChannel(ctx, channel, userID)
}

func (h *SessionHandler) loadSessionChannel(ctx context.Context, session model.Session) (model.Channel, bool, error) {
	var channel model.Channel
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", session.ChannelID, session.OrgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return channel, false, nil
	}
	return channel, err == nil, err
}

func (h *SessionHandler) canUseChannel(ctx context.Context, channel model.Channel, userID *uuid.UUID) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	orgRole, err := h.orgRole(ctx, channel.OrgID, userID)
	return err == nil && canUseChannel(ctx, h.db, channel, orgRole, userID, false)
}

func (h *SessionHandler) canViewChannel(ctx context.Context, channel model.Channel, userID *uuid.UUID) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	orgRole, err := h.orgRole(ctx, channel.OrgID, userID)
	return err == nil && canViewChannel(ctx, h.db, channel, orgRole, userID, false)
}

// canManageChannel reports whether the caller manages the session's channel — an
// org manager (owner/admin) or a manager of the channel's owning team. Used for
// the manager-oversight paths of session read/archive.
func (h *SessionHandler) canManageChannel(ctx context.Context, channel model.Channel, userID *uuid.UUID) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	orgRole, err := h.orgRole(ctx, channel.OrgID, userID)
	return err == nil && canManageChannel(ctx, h.db, channel, userID, orgRole, false)
}

func (h *SessionHandler) orgRole(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var membership model.OrgMembership
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, *userID).
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
		Where("channel_id = ? AND user_id = ? AND deactivated_at IS NULL", channelID, *userID).
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
