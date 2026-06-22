package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Create handles POST /v1/sessions.
// @Summary Create a session
// @Description Creates a channel-scoped session and optionally dispatches or queues the first user message.
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
// @Failure 502 {object} errorResponse
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
	raw := normalizeJSONPtr(&req.Raw)
	hasInitialMessage := sessionMessageHasContent(text, raw)
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
	if ok := h.validateSessionModel(w, r, org.ID, &agent, createSessionModelID(req)); !ok {
		return
	}
	if _, err := normalizeSessionReasoningEffort(createSessionReasoningEffort(req)); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	session := h.newSessionRecord(r, org.ID, channel.ID, agent, req, userID)
	var perSessionSandbox *model.Sandbox
	if agent.SandboxStrategy == agentStrategyPerSession {
		sb, err := h.provisionPerSessionSandbox(r.Context(), &agent)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "provision per-session sandbox for session create failed", "agent_id", agent.ID, "error", err)
			logging.Capture(r.Context(), err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to provision session sandbox"})
			return
		}
		perSessionSandbox = sb
		session.SandboxID = &sb.ID
	}
	queued := false
	if hasInitialMessage {
		intent, err := h.createInitialSessionMessageIntent(r.Context(), &session, userID, text, raw)
		if err != nil {
			if perSessionSandbox != nil {
				h.cleanupFailedPerSessionCreate(r.Context(), session.ID, perSessionSandbox)
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create session"})
			return
		}
		queued, err = h.dispatchSessionMessageIntent(r.Context(), intent)
		if err != nil {
			if agent.SandboxStrategy == agentStrategyPerSession {
				h.cleanupFailedPerSessionCreate(r.Context(), session.ID, perSessionSandbox)
				logging.FromContext(r.Context()).ErrorContext(r.Context(), "send initial per-session message failed", "session_id", session.ID, "agent_id", agent.ID, "error", err)
				logging.Capture(r.Context(), err)
				writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to send initial session message"})
				return
			}
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "send initial session message failed", "session_id", session.ID, "agent_id", agent.ID, "error", err)
			logging.Capture(r.Context(), err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to send initial session message"})
			return
		}
		if !queued {
			if err := h.db.WithContext(r.Context()).First(&session, "id = ?", session.ID).Error; err != nil {
				logging.FromContext(r.Context()).WarnContext(r.Context(), "reload session after initial delivery failed", "session_id", session.ID, "error", err)
			}
		}
	} else if err := h.createSessionOnly(r.Context(), &session, userID); err != nil {
		if perSessionSandbox != nil {
			h.cleanupFailedPerSessionCreate(r.Context(), session.ID, perSessionSandbox)
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create session"})
		return
	}
	if hasInitialMessage && strings.TrimSpace(req.Name) == "" {
		if err := h.enqueueSessionName(r.Context(), session.ID); err != nil {
			logging.FromContext(r.Context()).WarnContext(r.Context(), "enqueue session name task failed", "session_id", session.ID, "error", err)
		}
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusCreated, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Queued:  queued,
	})
}

func (h *SessionHandler) validateSessionModel(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, agent *model.Agent, requested string) bool {
	modelID := strings.TrimSpace(requested)
	if modelID == "" {
		modelID = strings.TrimSpace(agent.Model)
	}
	if modelID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent model is not configured"})
		return false
	}
	if !agentAllowsModel(agent, modelID) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "model is not enabled for this agent"})
		return false
	}
	agentHandler := AgentHandler{db: h.db}
	if err := agentHandler.validateAgentSelectableModel(r.Context(), orgID, modelID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return false
	}
	return true
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
		Preload("AgentCatalog").
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
	modelID := createSessionModelID(req)
	if modelID == "" {
		modelID = agent.Model
	}
	reasoningEffort, _ := normalizeSessionReasoningEffort(createSessionReasoningEffort(req))
	session := model.Session{
		ID:                sessionID,
		OrgID:             orgID,
		ChannelID:         channelID,
		AgentID:           agent.ID,
		SandboxID:         h.bestEffortSandboxID(r, orgID, agent),
		CreatedBy:         userID,
		Model:             modelID,
		AccessMode:        defaultString(strings.TrimSpace(req.AccessMode), "full"),
		ReasoningEffort:   reasoningEffort,
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
	return h.bestEffortSandboxIDForContext(r.Context(), orgID, agent)
}

func (h *SessionHandler) bestEffortSandboxIDForContext(ctx context.Context, orgID uuid.UUID, agent model.Agent) *uuid.UUID {
	if agent.SandboxStrategy != agentStrategyAlwaysOn {
		return nil
	}
	sandbox, err := agentsandbox.Selector{DB: h.db}.MainRuntime(ctx, orgID, agent.ID)
	if err != nil || sandbox == nil {
		return nil
	}
	return &sandbox.ID
}

func ptrSessionEventResponse(value sessionEventResponse) *sessionEventResponse {
	return &value
}
