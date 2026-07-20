package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/orgtier"
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
// @Failure 429 {object} errorResponse
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
	questionRequestID := strings.TrimSpace(req.QuestionRequestID)
	if questionRequestID == "" {
		questionRequestID = requestID
	}
	text := strings.TrimSpace(req.Text)
	optionID := strings.TrimSpace(req.OptionID)
	if len(req.Answers) > 0 {
		h.respondToStructuredInput(w, r, session, userID, questionRequestID, req.Answers)
		return
	}
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
	reservation := orgtier.WakeReservation{}
	if session.SandboxID != nil {
		var reserveErr error
		reservation, reserveErr = orgtier.ReserveSessionWake(r.Context(), h.db, session.OrgID, *session.SandboxID)
		if reserveErr != nil {
			if writeOrgTierError(w, reserveErr) {
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check session capacity"})
			return
		}
	}
	intent, err := h.createSessionMessageIntent(r.Context(), session, userID, messageText, payload, sessionMessageDeliveryOptions{
		ClearLastOutcome: true,
	})
	if err != nil {
		rollbackOrgTierWake(r.Context(), h.db, reservation)
		if errors.Is(err, errSessionSandboxDraining) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "agent sandbox is draining"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to queue input response"})
		return
	}
	queued, err := h.dispatchSessionMessageIntent(r.Context(), intent)
	if err != nil {
		rollbackOrgTierWake(r.Context(), h.db, reservation)
		if writeOrgTierError(w, err) {
			return
		}
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
	if intent.SkipDispatch || queued {
		rollbackOrgTierWake(r.Context(), h.db, reservation)
	} else {
		commitOrgTierWake(r.Context(), h.db, reservation)
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

func (h *SessionHandler) respondToStructuredInput(
	w http.ResponseWriter,
	r *http.Request,
	session model.Session,
	userID *uuid.UUID,
	questionRequestID string,
	answers map[string]sessionInputResponseAnswer,
) {
	if strings.TrimSpace(questionRequestID) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "question_request_id is required"})
		return
	}
	payload, err := h.questionAnswerPayload(r, userID, answers)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	reservation := orgtier.WakeReservation{}
	if session.SandboxID != nil {
		reservation, err = orgtier.ReserveSessionWake(r.Context(), h.db, session.OrgID, *session.SandboxID)
		if err != nil {
			if writeOrgTierError(w, err) {
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check session capacity"})
			return
		}
	}
	sb, err := h.sessionSandboxForAccess(r.Context(), &session)
	if err != nil {
		rollbackOrgTierWake(r.Context(), h.db, reservation)
		h.writeSessionSandboxUnavailableError(w, err)
		return
	}
	client, err := h.runtimeClientForSessionSandbox(r.Context(), sb)
	if err != nil {
		rollbackOrgTierWake(r.Context(), h.db, reservation)
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime question answer is not available"})
		return
	}
	if _, err := client.PostQuestionAnswer(r.Context(), session.ID.String(), questionRequestID, payload); err != nil {
		rollbackOrgTierWake(r.Context(), h.db, reservation)
		status, message := questionAnswerProxyError(err)
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	commitOrgTierWake(r.Context(), h.db, reservation)
	stats := h.statsForSessions(r.Context(), []uuid.UUID{session.ID})[session.ID]
	writeJSON(w, http.StatusAccepted, sessionMutationResponse{
		Session: sessionToResponse(session, stats.ParticipantCount, stats.EventCount, stats.LastEvent),
		Queued:  false,
	})
}

func (h *SessionHandler) questionAnswerPayload(r *http.Request, userID *uuid.UUID, answers map[string]sessionInputResponseAnswer) (agentruntime.QuestionAnswerPayload, error) {
	if len(answers) == 0 {
		return agentruntime.QuestionAnswerPayload{}, fmt.Errorf("answers are required")
	}
	out := make(map[string]agentruntime.QuestionAnswerValue, len(answers))
	for questionID, answer := range answers {
		id := strings.TrimSpace(questionID)
		if id == "" {
			return agentruntime.QuestionAnswerPayload{}, fmt.Errorf("answer question id is required")
		}
		if len(answer.Answers) != 1 {
			return agentruntime.QuestionAnswerPayload{}, fmt.Errorf("answer for %s must include one choice", id)
		}
		choice := strings.TrimSpace(answer.Answers[0])
		if choice == "" {
			return agentruntime.QuestionAnswerPayload{}, fmt.Errorf("answer for %s is required", id)
		}
		value := agentruntime.QuestionAnswerValue{Answers: []string{choice}}
		if answer.Other != nil {
			other := strings.TrimSpace(*answer.Other)
			if other != "" {
				value.Other = &other
			}
		}
		out[id] = value
	}
	payload := agentruntime.QuestionAnswerPayload{Answers: out}
	if userID != nil {
		user := userID.String()
		payload.User = &user
		if displayName := h.userDisplayName(r, *userID); displayName != "" {
			payload.UserDisplayName = &displayName
		}
	}
	return payload, nil
}

func (h *SessionHandler) userDisplayName(r *http.Request, userID uuid.UUID) string {
	var user model.User
	if err := h.db.WithContext(r.Context()).Select("name").First(&user, "id = ?", userID).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(user.Name)
}

func questionAnswerProxyError(err error) (int, string) {
	var statusErr *agentruntime.HTTPStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusBadRequest:
			return http.StatusBadRequest, "invalid question answer"
		case http.StatusNotFound:
			return http.StatusNotFound, "question request not found"
		case http.StatusConflict:
			return http.StatusConflict, "question request cannot be answered"
		}
	}
	return http.StatusBadGateway, "failed to answer question"
}
