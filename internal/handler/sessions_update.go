package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/channelagents"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
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
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
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
	// A PATCH that archives the session tears down its sandbox, same as DELETE.
	if req.Status != nil && strings.TrimSpace(*req.Status) == "archived" {
		h.enqueueSessionSandboxTeardown(r.Context(), &session)
	}
	// Archiving or ending a session triggers a final reflection pass over any
	// unreflected event tail.
	if req.Status != nil {
		if status := strings.TrimSpace(*req.Status); status == "archived" || status == "ended" {
			_ = tasks.EnqueueSessionReflection(r.Context(), h.enqueuer, session.ID)
		}
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusOK, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
	})
}

func (h *SessionHandler) applySessionUpdates(w http.ResponseWriter, r *http.Request, session *model.Session, userID *uuid.UUID, req updateSessionRequest, updates map[string]any) bool {
	finalChannel := model.Channel{ID: session.ChannelID, OrgID: session.OrgID, DefaultAgentID: session.AgentID}
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
		if status == "archived" && !h.requireSessionArchivePermission(w, r, *session, userID) {
			return false
		}
		if status == "archived" && !h.requireSessionIdleForArchive(w, *session) {
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
		finalChannel = channel
		// The session keeps its current agent unless the caller reassigns it, so the
		// existing agent must be allowed to act in the destination channel under the
		// team-primary rule (channelagents.ActsInChannel). This is the same gate
		// session create and agent reassignment apply — moving a session must not be
		// a back door into a channel whose team excludes the agent.
		if req.AgentID == nil {
			acts, err := channelagents.ActsInChannel(r.Context(), h.db, session.OrgID, finalChannel.ID, session.AgentID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check channel agents"})
				return false
			}
			if !acts {
				writeJSON(w, http.StatusForbidden, errorResponse{Error: "agent does not belong to this channel's team"})
				return false
			}
		}
	}
	if req.AgentID != nil {
		agent, ok := h.resolveSessionAgent(w, r, session.OrgID, finalChannel, *req.AgentID)
		if !ok {
			return false
		}
		updates["agent_id"] = agent.ID
		session.AgentID = agent.ID
		_ = h.writeSystemEvent(r, session, userID, "session.agent_reassigned", model.JSON{
			"agent_id": agent.ID.String(),
		})
	}
	if req.ImageModel != nil {
		value := strings.TrimSpace(*req.ImageModel)
		if err := validateImageModelPreference(value, false); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["image_model"] = value
		session.ImageModel = value
	}
	if req.VectorImageModel != nil {
		value := strings.TrimSpace(*req.VectorImageModel)
		if err := validateImageModelPreference(value, true); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["vector_image_model"] = value
		session.VectorImageModel = value
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
