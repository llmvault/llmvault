package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/model"
)

// Create handles POST /v1/sessions.
// @Summary Create a session
// @Description Creates a channel-scoped session and queues the first user message.
// @Tags sessions
// @Accept json
// @Produce json
// @Param body body createSessionRequest true "Session create payload"
// @Success 201 {object} sessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions [post]
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := sessionOrg(w, r)
	if !ok {
		return
	}
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	text := strings.TrimSpace(firstNonEmptyString(req.Text, req.Message))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "text is required"})
		return
	}
	channel, ok := h.loadUsableChannel(w, r, org.ID, req.ChannelID)
	if !ok {
		return
	}
	userID, _ := currentSessionUserID(r)
	if !h.canUseChannel(r.Context(), channel, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "join the channel before creating sessions"})
		return
	}
	agent, ok := h.resolveSessionAgent(w, r, org.ID, channel, req.AgentID)
	if !ok {
		return
	}
	session := h.newSessionRecord(r, org.ID, channel.ID, agent, req, userID)
	var event model.SessionEvent
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if userID != nil {
			now := time.Now()
			if err := tx.Create(&model.SessionParticipant{
				SessionID: session.ID,
				UserID:    *userID,
				Role:      "owner",
				JoinedAt:  &now,
			}).Error; err != nil {
				return err
			}
		}
		var err error
		event, err = h.createUserMessageEvent(tx, &session, userID, text, normalizeJSONPtr(&req.Raw))
		return err
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create session"})
		return
	}
	queued, err := h.dispatchOrQueueSessionDelivery(r.Context(), session.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue session delivery"})
		return
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusCreated, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:   ptrSessionEventResponse(eventToResponse(event)),
		Queued:  queued,
	})
}

func (h *SessionHandler) loadUsableChannel(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, raw string) (model.Channel, bool) {
	channelID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel_id must be a uuid"})
		return model.Channel{}, false
	}
	var channel model.Channel
	err = h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "channel not found"})
		return model.Channel{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channel"})
		return model.Channel{}, false
	}
	return channel, true
}

func (h *SessionHandler) resolveSessionAgent(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, channel model.Channel, raw string) (model.Agent, bool) {
	agentID := channel.DefaultAgentID
	if strings.TrimSpace(raw) != "" {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
			return model.Agent{}, false
		}
		agentID = id
	}
	var agent model.Agent
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return model.Agent{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return model.Agent{}, false
	}
	return agent, true
}

func (h *SessionHandler) newSessionRecord(r *http.Request, orgID, channelID uuid.UUID, agent model.Agent, req createSessionRequest, userID *uuid.UUID) model.Session {
	sessionID := uuid.New()
	modelID := strings.TrimSpace(req.Model)
	if modelID == "" {
		modelID = agent.Model
	}
	session := model.Session{
		ID:                sessionID,
		OrgID:             orgID,
		ChannelID:         channelID,
		AgentID:           agent.ID,
		SandboxID:         h.bestEffortSandboxID(r, orgID, agent),
		CreatedBy:         userID,
		Model:             modelID,
		AccessMode:        defaultString(strings.TrimSpace(req.AccessMode), "full"),
		ReasoningEffort:   defaultString(strings.TrimSpace(req.ReasoningEffort), "high"),
		Source:            "web",
		SourceResourceKey: sessionID.String(),
		Name:              defaultString(strings.TrimSpace(req.Name), webSessionName(firstNonEmptyString(req.Text, req.Message))),
		Status:            "active",
		AgentTurnStatus:   model.SessionAgentTurnIdle,
		IntegrationScopes: model.JSON{},
	}
	return session
}

func (h *SessionHandler) bestEffortSandboxID(r *http.Request, orgID uuid.UUID, agent model.Agent) *uuid.UUID {
	if agent.SandboxStrategy != agentStrategyAlwaysOn {
		return nil
	}
	sandbox, err := agentsandbox.Selector{DB: h.db}.MainRuntime(r.Context(), orgID, agent.ID)
	if err != nil || sandbox == nil {
		return nil
	}
	return &sandbox.ID
}

func ptrSessionEventResponse(value sessionEventResponse) *sessionEventResponse {
	return &value
}
