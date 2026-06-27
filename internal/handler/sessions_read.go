package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const sessionSortActivity = "activity"

// List handles GET /v1/sessions.
// @Summary List sessions
// @Description Lists sessions visible to the caller, optionally filtered by channel or agent.
// @Tags sessions
// @Produce json
// @Param channel_id query string false "Channel ID"
// @Param agent_id query string false "Agent ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Param sort query string false "Sort order: created_at or activity"
// @Success 200 {object} paginatedResponse[sessionResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions [get]
func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	h.listSessions(w, r, uuid.Nil)
}

// ListChannelSessions handles GET /v1/channels/{id}/sessions.
// @Summary List channel sessions
// @Description Lists sessions in a visible channel.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Param sort query string false "Sort order: created_at or activity"
// @Success 200 {object} paginatedResponse[sessionResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/sessions [get]
func (h *SessionHandler) ListChannelSessions(w http.ResponseWriter, r *http.Request) {
	channelID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel id"})
		return
	}
	h.listSessions(w, r, channelID)
}

// Get handles GET /v1/sessions/{id}.
// @Summary Get a session
// @Description Returns one visible session and its participants.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} sessionDetailResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id} [get]
func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusOK, sessionDetailResponse{
		Session:      sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Participants: h.participants(r.Context(), session.ID),
	})
}

func (h *SessionHandler) listSessions(w http.ResponseWriter, r *http.Request, forcedChannelID uuid.UUID) {
	org, ok := sessionOrg(w, r)
	if !ok {
		return
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "created_at"
	}
	if sort != "created_at" && sort != sessionSortActivity {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sort must be created_at or activity"})
		return
	}
	limit, cursor, activityCursor, err := parseSessionListPagination(r, sort)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	userID, _ := currentSessionUserID(r)
	query := h.db.WithContext(r.Context()).
		Model(&model.Session{}).
		Where("org_id = ? AND status <> ?", org.ID, "archived")
	channelScoped := false
	if forcedChannelID != uuid.Nil {
		if !h.canListChannelSessions(w, r, org.ID, forcedChannelID, userID) {
			return
		}
		query = query.Where("channel_id = ?", forcedChannelID)
		channelScoped = true
	} else if channelID := strings.TrimSpace(r.URL.Query().Get("channel_id")); channelID != "" {
		id, err := uuid.Parse(channelID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_id must be a uuid"})
			return
		}
		if !h.canListChannelSessions(w, r, org.ID, id, userID) {
			return
		}
		query = query.Where("channel_id = ?", id)
		channelScoped = true
	}
	if agentID := strings.TrimSpace(r.URL.Query().Get("agent_id")); agentID != "" {
		id, err := uuid.Parse(agentID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
			return
		}
		query = query.Where("agent_id = ?", id)
	}
	query = h.applySessionListVisibility(r, query, org.ID, userID, sort, channelScoped)
	if sort == sessionSortActivity {
		query = applySessionActivityPagination(query, activityCursor, limit)
	} else {
		query = applyPagination(query, cursor, limit)
	}
	var sessions []model.Session
	if err := query.Find(&sessions).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list sessions"})
		return
	}
	hasMore := len(sessions) > limit
	if hasMore {
		sessions = sessions[:limit]
	}
	stats := h.statsForSessions(r.Context(), sessionIDsV2(sessions))
	items := sessionResponsesFromStats(sessions, stats)
	resp := paginatedResponse[sessionResponse]{Data: items, HasMore: hasMore}
	if hasMore {
		last := sessions[len(sessions)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		if sort == sessionSortActivity {
			next = encodeActivityCursor(sessionActivityTime(last, stats[last.ID]), last.CreatedAt, last.ID)
		}
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *SessionHandler) canListChannelSessions(w http.ResponseWriter, r *http.Request, orgID, channelID uuid.UUID, userID *uuid.UUID) bool {
	var channel model.Channel
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "channel not found"})
		return false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channel"})
		return false
	}
	if !h.canViewChannel(r.Context(), channel, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "channel access denied"})
		return false
	}
	return true
}

func (h *SessionHandler) isOrgAdminForSession(r *http.Request, orgID uuid.UUID, userID *uuid.UUID) bool {
	role, err := h.orgRole(r.Context(), orgID, userID)
	return err == nil && isOrgManager(role)
}

func (h *SessionHandler) applySessionListVisibility(r *http.Request, query *gorm.DB, orgID uuid.UUID, userID *uuid.UUID, sort string, channelScoped bool) *gorm.DB {
	if isAPIKeyRequest(r.Context()) {
		return query
	}
	if userID == nil {
		return query.Where("1 = 0")
	}
	if sort != sessionSortActivity && h.isOrgAdminForSession(r, orgID, userID) {
		return query
	}
	return h.applyOwnedParticipantSessionFilter(query, userID, channelScoped)
}

func (h *SessionHandler) applyOwnedParticipantSessionFilter(query *gorm.DB, userID *uuid.UUID, includeExternalSource bool) *gorm.DB {
	if userID == nil {
		return query.Where("1 = 0")
	}
	if includeExternalSource {
		return query.Where("created_by = ? OR id IN (?) OR source = ?", *userID, h.participantSessionSubquery(userID), model.SessionSourceExternal)
	}
	return query.Where("created_by = ? OR id IN (?)", *userID, h.participantSessionSubquery(userID))
}

func (h *SessionHandler) participantSessionSubquery(userID *uuid.UUID) *gorm.DB {
	q := h.db.Model(&model.SessionParticipant{}).Select("session_id")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("user_id = ?", *userID)
}

func (h *SessionHandler) memberChannelSubquery(userID *uuid.UUID) *gorm.DB {
	q := h.db.Model(&model.ChannelMember{}).Select("channel_id")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("user_id = ?", *userID)
}

func (h *SessionHandler) sessionListResponses(r *http.Request, sessions []model.Session) []sessionResponse {
	ids := sessionIDsV2(sessions)
	stats := h.statsForSessions(r.Context(), ids)
	return sessionResponsesFromStats(sessions, stats)
}
