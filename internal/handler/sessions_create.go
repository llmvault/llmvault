package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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
	ctx := r.Context()
	totalStarted := time.Now()
	phaseStarted := totalStarted
	logPhase := func(phase string, attrs ...any) {
		attrs = append(attrs,
			"total_ms", time.Since(totalStarted).Milliseconds(),
		)
		logging.LogPhase(ctx, "session create phase", phase, phaseStarted, attrs...)
		phaseStarted = time.Now()
	}
	org, ok := sessionOrg(w, r)
	if !ok {
		return
	}
	var req createSessionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	text := strings.TrimSpace(req.Text)
	payload := model.JSON{}
	hasInitialMessage := text != ""
	logPhase("decode request",
		"org_id", org.ID,
		"channel_id", strings.TrimSpace(req.ChannelID),
		"requested_agent_id", strings.TrimSpace(req.AgentID),
		"has_initial_message", hasInitialMessage,
		"requested_model", createSessionModelID(req),
		"has_reasoning_effort", createSessionReasoningEffort(req) != "",
	)
	channel, ok := h.loadUsableChannel(w, r, org.ID, req.ChannelID)
	if !ok {
		return
	}
	logPhase("load channel", "org_id", org.ID, "channel_id", channel.ID)
	userID, _ := currentSessionUserID(r)
	if !h.canUseChannel(ctx, channel, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "join the channel before creating sessions"})
		return
	}
	logPhase("authorize channel", "org_id", org.ID, "channel_id", channel.ID, "user_id", userID)
	agent, ok := h.resolveSessionAgent(w, r, org.ID, channel, req.AgentID)
	if !ok {
		return
	}
	logPhase("resolve agent", "org_id", org.ID, "channel_id", channel.ID, "agent_id", agent.ID, "sandbox_strategy", agent.SandboxStrategy)
	if ok := h.validateSessionModel(w, r, org.ID, &agent, createSessionModelID(req)); !ok {
		return
	}
	if _, err := normalizeSessionReasoningEffort(createSessionReasoningEffort(req)); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	logPhase("validate runtime options", "org_id", org.ID, "agent_id", agent.ID)
	session := h.newSessionRecord(r, org.ID, channel.ID, agent, req, userID)
	logPhase("build session record", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID, "sandbox_strategy", agent.SandboxStrategy)
	var perSessionSandbox *model.Sandbox
	if agent.SandboxStrategy == agentStrategyPerSession {
		sb, err := h.provisionPerSessionSandbox(ctx, &agent, session.Model, session.ReasoningEffort)
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "provision per-session sandbox for session create failed", "agent_id", agent.ID, "error", err)
			logging.Capture(ctx, err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to provision session sandbox"})
			return
		}
		perSessionSandbox = sb
		session.SandboxID = &sb.ID
		logPhase("provision per-session sandbox", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID, "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	}
	queued := false
	var event *model.SessionEvent
	if hasInitialMessage {
		intent, err := h.createInitialSessionMessageIntentWithOptions(ctx, &session, userID, text, payload, sessionMessageDeliveryOptions{})
		if err != nil {
			if perSessionSandbox != nil {
				h.cleanupFailedPerSessionCreate(ctx, session.ID, perSessionSandbox)
			}
			if errors.Is(err, errSessionSandboxDraining) {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "agent sandbox is draining"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create session"})
			return
		}
		event = intent.Event
		logPhase("create initial message intent", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID, "event_created", intent.EventCreated, "queued", intent.Queued)
		queued, err = h.dispatchSessionMessageIntent(ctx, intent)
		if err != nil {
			if errors.Is(err, errSessionSandboxDraining) {
				if perSessionSandbox != nil {
					h.cleanupFailedPerSessionCreate(ctx, session.ID, perSessionSandbox)
				}
				writeJSON(w, http.StatusConflict, errorResponse{Error: "agent sandbox is draining"})
				return
			}
			if agent.SandboxStrategy == agentStrategyPerSession {
				h.cleanupFailedPerSessionCreate(ctx, session.ID, perSessionSandbox)
				logging.FromContext(ctx).ErrorContext(ctx, "send initial per-session message failed", "session_id", session.ID, "agent_id", agent.ID, "error", err)
				logging.Capture(ctx, err)
				writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to send initial session message"})
				return
			}
			logging.FromContext(ctx).ErrorContext(ctx, "send initial session message failed", "session_id", session.ID, "agent_id", agent.ID, "error", err)
			logging.Capture(ctx, err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to send initial session message"})
			return
		}
		logPhase("dispatch initial message", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID, "queued", queued)
		if !queued {
			if err := h.db.WithContext(ctx).First(&session, "id = ?", session.ID).Error; err != nil {
				logging.FromContext(ctx).WarnContext(ctx, "reload session after initial delivery failed", "session_id", session.ID, "error", err)
			}
			logPhase("reload session", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID)
		}
	} else if err := h.createSessionOnly(ctx, &session, userID); err != nil {
		if perSessionSandbox != nil {
			h.cleanupFailedPerSessionCreate(ctx, session.ID, perSessionSandbox)
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create session"})
		return
	} else {
		logPhase("create session row", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID)
	}
	if hasInitialMessage {
		if err := h.enqueueSessionName(ctx, session.ID); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "enqueue session name task failed", "session_id", session.ID, "error", err)
		}
		logPhase("enqueue session name", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID)
	}
	stats := h.statsForSessions(ctx, []uuid.UUID{session.ID})[session.ID]
	logPhase("load response stats", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID, "queued", queued)
	writeJSON(w, http.StatusCreated, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:   sessionMutationEventResponse(event),
		Queued:  queued,
	})
	logPhase("complete", "org_id", org.ID, "agent_id", agent.ID, "session_id", session.ID, "queued", queued)
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
	if !agentAllowedInChannel(r.Context(), h.db, orgID, agent.ID, channel.ID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "agent is not available in this channel"})
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
		ReasoningEffort:   reasoningEffort,
		Source:            "web",
		SourceResourceKey: sessionID.String(),
		Name:              webSessionName(req.Text),
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
