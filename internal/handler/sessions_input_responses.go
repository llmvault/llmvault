package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// RespondToInput handles POST /v1/sessions/{id}/input-responses.
// @Summary Respond to a pending agent input request
// @Description Sends an answer command to a runtime request_user_input turn.
// @Tags sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body sessionInputResponseRequest true "Input response payload"
// @Success 202 {object} sessionMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/input-responses [post]
func (h *SessionHandler) RespondToInput(w http.ResponseWriter, r *http.Request) {
	session, userID, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	if session.Status != "active" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session is not active"})
		return
	}
	var req sessionInputResponseRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	requestID := strings.TrimSpace(req.RequestID)
	text := strings.TrimSpace(req.Text)
	optionID := strings.TrimSpace(req.OptionID)
	if requestID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "request_id is required"})
		return
	}
	if text == "" && optionID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "text or option_id is required"})
		return
	}
	messageText := text
	if messageText == "" {
		messageText = optionID
	}
	payload := model.JSON{
		"input_response": map[string]any{
			"request_id": requestID,
			"text":       text,
			"option_id":  optionID,
		},
	}
	intent, err := h.createSessionMessageIntent(r.Context(), session, userID, messageText, payload, sessionMessageDeliveryOptions{
		ClearLastOutcome: true,
	})
	if err != nil {
		if errors.Is(err, errSessionSandboxDraining) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent sandbox is draining"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue input response"})
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
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to send input response"})
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
