package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// SendMessage handles POST /v1/sessions/{id}/messages.
// @Summary Send a session message
// @Description Persists a user message and queues it for FIFO runtime delivery.
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
	session, userID, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	if session.Status != "active" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session is not active"})
		return
	}
	var req sendSessionMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	text := strings.TrimSpace(firstNonEmptyString(req.Text, req.Message))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "text is required"})
		return
	}
	var selectedModel string
	var selectedReasoningEffort string
	if req.ModelDefinition != nil {
		selectedModel = strings.TrimSpace(req.ModelDefinition.ModelID)
		if selectedModel == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "model_definition.model_id is required"})
			return
		}
		var agent model.Agent
		loadErr := h.db.WithContext(r.Context()).
			Where("id = ? AND org_id = ? AND status <> ?", session.AgentID, session.OrgID, "archived").
			First(&agent).Error
		if loadErr != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
			return
		}
		if ok := h.validateSessionModel(w, r, session.OrgID, &agent, selectedModel); !ok {
			return
		}
		selectedReasoningEffort = strings.TrimSpace(req.ModelDefinition.ReasoningEffort)
		if selectedReasoningEffort == "" {
			selectedReasoningEffort = session.ReasoningEffort
		}
		var err error
		selectedReasoningEffort, err = normalizeSessionReasoningEffort(selectedReasoningEffort)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
	}
	payload := normalizeJSONPtr(&req.Raw)
	if req.User != "" {
		payload["user"] = strings.TrimSpace(req.User)
	}
	if req.UserDisplayName != "" {
		payload["user_display_name"] = strings.TrimSpace(req.UserDisplayName)
	}
	if len(req.DynamicContext) > 0 {
		payload["dynamic_context"] = req.DynamicContext
	}
	if selectedModel != "" {
		payload["model_definition"] = map[string]any{
			"model_id":         selectedModel,
			"reasoning_effort": selectedReasoningEffort,
		}
	}
	var event model.SessionEvent
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		event, err = h.createUserMessageEvent(tx, &session, userID, text, payload)
		if err != nil {
			return err
		}
		updates := map[string]any{"updated_at": event.EventAt}
		if selectedModel != "" {
			updates["model"] = selectedModel
			updates["reasoning_effort"] = selectedReasoningEffort
		}
		return tx.Model(&model.Session{}).Where("id = ?", session.ID).Updates(updates).Error
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue session message"})
		return
	}
	queued, err := h.dispatchOrQueueSessionDelivery(r.Context(), session.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue session delivery"})
		return
	}
	session.UpdatedAt = event.EventAt
	if selectedModel != "" {
		session.Model = selectedModel
		session.ReasoningEffort = selectedReasoningEffort
	}
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusAccepted, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:   ptrSessionEventResponse(eventToResponse(event)),
		Queued:  queued,
	})
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
