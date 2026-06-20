package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

// RespondToInput handles POST /v1/sessions/{id}/input-responses.
// @Summary Respond to a pending agent input request
// @Description Persists an answer to a runtime request_user_input turn and queues it for runtime delivery.
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	var event model.SessionEvent
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := h.createQuestionAnsweredEvent(tx, &session, userID, requestID, text, optionID); err != nil {
			return err
		}
		var err error
		event, err = h.createUserMessageEvent(tx, &session, userID, messageText, payload)
		if err != nil {
			return err
		}
		return tx.Model(&model.Session{}).Where("id = ?", session.ID).Updates(map[string]any{
			"updated_at":              event.EventAt,
			"agent_turn_last_outcome": "",
		}).Error
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue input response"})
		return
	}
	queued, err := h.dispatchOrQueueSessionDelivery(r.Context(), session.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue session delivery"})
		return
	}
	session.UpdatedAt = event.EventAt
	session.AgentTurnLastOutcome = ""
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusAccepted, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Event:   ptrSessionEventResponse(eventToResponse(event)),
		Queued:  queued,
	})
}

func (h *SessionHandler) createQuestionAnsweredEvent(tx *gorm.DB, session *model.Session, actor *uuid.UUID, requestID, text, optionID string) error {
	seq, err := h.nextSessionSequence(tx, session.ID)
	if err != nil {
		return err
	}
	event := model.SessionEvent{
		OrgID:            session.OrgID,
		SessionID:        session.ID,
		AgentID:          session.AgentID,
		SandboxID:        session.SandboxID,
		RuntimeSessionID: session.ID.String(),
		EventID:          "question-answered-" + uuid.NewString(),
		EventType:        runtimeevents.EventQuestionAnswered,
		ActorUserID:      actor,
		Source:           defaultString(session.Source, "web"),
		SequenceNumber:   seq,
		Payload: model.JSON{
			"request_id":  requestID,
			"text":        text,
			"option_id":   optionID,
			"answered_at": time.Now().UTC().Format(time.RFC3339Nano),
		},
		EventAt: time.Now(),
	}
	return tx.Create(&event).Error
}
