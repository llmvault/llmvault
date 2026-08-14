package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type desktopRuntimeConfigResponse struct {
	AgentID   string                           `json:"agent_id"`
	SandboxID string                           `json:"sandbox_id"`
	Config    agentruntime.ConfigUpdateRequest `json:"config"`
}

type desktopSessionMutationResponse struct {
	Session        sessionResponse                  `json:"session"`
	Event          *sessionEventResponse            `json:"event,omitempty"`
	RuntimeRequest *agentruntime.HTTPMessageRequest `json:"runtime_request,omitempty"`
}

type desktopDeliveryRequest struct {
	StreamID string `json:"stream_id"`
	TurnID   string `json:"turn_id"`
}

// CreateDesktopSession creates a normal cloud-visible session without
// provisioning or dispatching to a hosted sandbox. When an initial message is
// provided, the response includes the exact runtime request for local delivery.
// @Summary Create a desktop-executed session
// @Tags desktop
// @Accept json
// @Produce json
// @Param body body createSessionRequest true "Desktop session payload"
// @Success 201 {object} desktopSessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/desktop/sessions [post]
func (h *SessionHandler) CreateDesktopSession(w http.ResponseWriter, r *http.Request) {
	org, ok := sessionOrg(w, r)
	if !ok {
		return
	}
	userID, ok := currentSessionUserID(r)
	if !ok || userID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	var req createSessionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	agent, ok := h.resolveSessionAgent(w, r, org.ID, req.AgentID)
	if !ok {
		return
	}
	if !h.canUseTeam(r.Context(), org.ID, agent.TeamID, userID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return
	}
	if ok := h.validateSessionModel(w, r, org.ID, &agent, createSessionModelID(req)); !ok {
		return
	}
	sb, ok := h.loadDesktopSandbox(w, r, org.ID, agent.ID, *userID)
	if !ok {
		return
	}
	session := h.newSessionRecord(r, org.ID, agent.TeamID, agent, req, userID)
	session.SandboxID = &sb.ID
	if err := h.createSessionOnly(r.Context(), &session, userID); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create desktop session", "org_id", org.ID, "agent_id", agent.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create desktop session"})
		return
	}

	var event *model.SessionEvent
	var runtimeRequest *agentruntime.HTTPMessageRequest
	text := strings.TrimSpace(req.Text)
	if sessionMessageHasContent(text, sessionMessageRequestPayload(sendSessionMessageRequest{AttachmentIDs: req.AttachmentIDs})) {
		payload, hydrated := h.hydrateSessionMessageAttachmentsForRequest(w, r, session, sessionMessageRequestPayload(sendSessionMessageRequest{AttachmentIDs: req.AttachmentIDs}))
		if !hydrated {
			h.deleteDesktopSession(r.Context(), session.ID)
			return
		}
		intent, err := h.createSessionMessageIntent(r.Context(), session, userID, text, payload, sessionMessageDeliveryOptions{})
		if err != nil {
			h.deleteDesktopSession(r.Context(), session.ID)
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "prepare desktop initial message", "session_id", session.ID, "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to prepare desktop session message"})
			return
		}
		session = intent.Session
		event = intent.Event
		message := tasks.RuntimeMessageFromCommand(session, intent.Command)
		runtimeRequest = &message
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusCreated, desktopSessionMutationResponse{
		Session:        sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:          sessionMutationEventResponse(event),
		RuntimeRequest: runtimeRequest,
	})
}

// SendDesktopSessionMessage persists and hydrates a cloud transcript event and
// returns its local-runtime delivery request without contacting a cloud sandbox.
// @Summary Prepare a desktop session message
// @Tags desktop
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body sendSessionMessageRequest true "Message payload"
// @Success 202 {object} desktopSessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/desktop/sessions/{id}/messages [post]
func (h *SessionHandler) SendDesktopSessionMessage(w http.ResponseWriter, r *http.Request) {
	session, userID, ok := h.authorizeDesktopSession(w, r)
	if !ok {
		return
	}
	var req sendSessionMessageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	text := strings.TrimSpace(req.Text)
	payload := sessionMessageRequestPayload(req)
	if !sessionMessageHasContent(text, payload) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "text is required"})
		return
	}
	payload, hydrated := h.hydrateSessionMessageAttachmentsForRequest(w, r, session, payload)
	if !hydrated {
		return
	}
	intent, err := h.createSessionMessageIntent(r.Context(), session, userID, text, payload, sessionMessageDeliveryOptions{})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "prepare desktop session message", "session_id", session.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to prepare desktop session message"})
		return
	}
	message := tasks.RuntimeMessageFromCommand(intent.Session, intent.Command)
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusAccepted, desktopSessionMutationResponse{
		Session:        sessionToResponse(intent.Session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:          sessionMutationEventResponse(intent.Event),
		RuntimeRequest: &message,
	})
}

// RecordDesktopDelivery records the local runtime's turn identity so existing
// cloud SSE and session status behavior remains identical to hosted execution.
// @Summary Record a desktop runtime delivery
// @Tags desktop
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body desktopDeliveryRequest true "Local runtime delivery"
// @Success 200 {object} sessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/desktop/sessions/{id}/delivery [post]
func (h *SessionHandler) RecordDesktopDelivery(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeDesktopSession(w, r)
	if !ok {
		return
	}
	var req desktopDeliveryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || strings.TrimSpace(req.TurnID) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "turn_id is required"})
		return
	}
	if err := h.recordDirectSessionDelivery(r.Context(), session.ID, &agentruntime.HTTPMessageResponse{
		StreamID: strings.TrimSpace(req.StreamID),
		TurnID:   strings.TrimSpace(req.TurnID),
	}); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "record desktop runtime delivery", "session_id", session.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to record desktop runtime delivery"})
		return
	}
	if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ?", session.ID, session.OrgID).First(&session).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load desktop session"})
		return
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusOK, sessionMutationResponse{Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent)})
}
