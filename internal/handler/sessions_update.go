package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Update handles PATCH /v1/sessions/{id}.
// @Summary Update a session
// @Description Renames, archives, moves, or reassigns a visible session.
// @Tags sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body updateSessionRequest true "Session update payload"
// @Success 200 {object} sessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id} [patch]
func (h *SessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	session, userID, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	var req updateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	updates := map[string]any{}
	if !h.applySessionUpdates(w, r, &session, userID, req, updates) {
		return
	}
	if len(updates) > 0 {
		if err := h.db.WithContext(r.Context()).
			Model(&model.Session{}).
			Where("id = ?", session.ID).
			Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update session"})
			return
		}
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusOK, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
	})
}

func (h *SessionHandler) applySessionUpdates(w http.ResponseWriter, r *http.Request, session *model.Session, userID *uuid.UUID, req updateSessionRequest, updates map[string]any) bool {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		updates["name"] = name
		session.Name = name
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != "active" && status != "archived" && status != "ended" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "status must be active, archived, or ended"})
			return false
		}
		updates["status"] = status
		session.Status = status
		if status == "archived" || status == "ended" {
			now := time.Now()
			updates["ended_at"] = &now
			session.EndedAt = &now
		}
	}
	if req.ChannelID != nil {
		channel, ok := h.loadUsableChannel(w, r, session.OrgID, *req.ChannelID)
		if !ok {
			return false
		}
		if !h.canUseChannel(r.Context(), channel, userID) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "channel access denied"})
			return false
		}
		updates["channel_id"] = channel.ID
		session.ChannelID = channel.ID
	}
	if req.AgentID != nil {
		agent, ok := h.resolveSessionAgent(w, r, session.OrgID, model.Channel{DefaultAgentID: session.AgentID}, *req.AgentID)
		if !ok {
			return false
		}
		updates["agent_id"] = agent.ID
		session.AgentID = agent.ID
		_ = h.writeSystemEvent(r, session, userID, "session.agent_reassigned", model.JSON{
			"agent_id": agent.ID.String(),
		})
	}
	return true
}

func (h *SessionHandler) writeSystemEvent(r *http.Request, session *model.Session, actor *uuid.UUID, eventType string, payload model.JSON) error {
	event := model.SessionEvent{
		OrgID:            session.OrgID,
		SessionID:        session.ID,
		AgentID:          session.AgentID,
		SandboxID:        session.SandboxID,
		RuntimeSessionID: session.ID.String(),
		EventID:          "system-" + uuid.NewString(),
		EventType:        eventType,
		ActorUserID:      actor,
		Source:           "web",
		Payload:          payload,
		EventAt:          time.Now(),
	}
	return h.db.WithContext(r.Context()).Create(&event).Error
}
