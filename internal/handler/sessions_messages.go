package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// SendMessage handles POST /v1/sessions/{id}/messages.
// @Summary Send a session message
// @Description Queues a user message command for FIFO runtime delivery.
// @Tags sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body sendSessionMessageRequest true "Message payload"
// @Success 202 {object} sessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/messages [post]
func (h *SessionHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	session, userID, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	if h.rejectExternalSessionMessage(w, r, session) {
		return
	}
	if !h.canAccessSession(r.Context(), session, userID, true) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "session access denied"})
		return
	}
	if session.Status != "active" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session is not active"})
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
	var hydrated bool
	payload, hydrated = h.hydrateSessionMessageAttachmentsForRequest(w, r, session, payload)
	if !hydrated {
		return
	}
	intent, err := h.createSessionMessageIntent(r.Context(), session, userID, text, payload, sessionMessageDeliveryOptions{})
	if err != nil {
		if errors.Is(err, errSessionSandboxDraining) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent sandbox is draining"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue session message"})
		return
	}
	queued, err := h.dispatchSessionMessageIntent(r.Context(), intent)
	if err != nil {
		if errors.Is(err, errSessionSandboxDraining) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent sandbox is draining"})
			return
		}
		if intent.Queued {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue session delivery"})
			return
		}
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to send session message"})
		return
	}
	session = intent.Session
	if !queued {
		if err := h.db.WithContext(r.Context()).First(&session, "id = ?", session.ID).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session"})
			return
		}
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusAccepted, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:   sessionMutationEventResponse(intent.Event),
		Queued:  queued,
	})
}

func (h *SessionHandler) rejectExternalSessionMessage(w http.ResponseWriter, r *http.Request, session model.Session) bool {
	external, provider := h.externalSessionMessageTarget(r, session)
	if !external {
		return false
	}
	target := "an external provider"
	if provider != "" {
		target = "external provider " + provider
	}
	writeJSON(w, http.StatusConflict, errorResponse{
		Error: "This session was initiated from " + target + ". Please continue the conversation there.",
	})
	return true
}

func (h *SessionHandler) externalSessionMessageTarget(r *http.Request, session model.Session) (bool, string) {
	// Gate only on where the session was initiated, not on whether the channel
	// is linked to an external provider: a web-initiated session on a
	// Slack-connected channel is a normal web session and must not be told to
	// "continue in Slack". The channel is loaded solely to label the provider.
	external := session.Source == model.SessionSourceExternal
	provider := ""
	if !external {
		return false, ""
	}
	if provider == "" {
		provider = externalProviderLabel(externalProviderFromResourceKey(session.SourceResourceKey))
	}
	return true, provider
}

func externalProviderFromResourceKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ":")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func externalProviderLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "":
		return ""
	case "slack":
		return "Slack"
	case "microsoft-teams", "msteams", "teams":
		return "Microsoft Teams"
	default:
		return strings.TrimSpace(provider)
	}
}

// ListEvents handles GET /v1/sessions/{id}/events.
// @Summary List session events
// @Description Lists visible session transcript events.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[sessionEventResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/events [get]
func (h *SessionHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	query := h.db.WithContext(r.Context()).
		Where("session_id = ?", session.ID)
	query = applyPagination(query, cursor, limit)
	var events []model.SessionEvent
	if err := query.Find(&events).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list session events"})
		return
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	out := make([]sessionEventResponse, len(events))
	for i, event := range events {
		out[i] = eventToResponse(event)
	}
	resp := paginatedResponse[sessionEventResponse]{Data: out, HasMore: hasMore}
	if hasMore {
		last := events[len(events)-1]
		next := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, resp)
}
