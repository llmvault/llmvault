package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/middleware"
)

const webSessionSource = "web"

type sendEmployeeSessionMessageRequest struct {
	Text      string `json:"text"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

type sendEmployeeSessionMessageResponse struct {
	EmployeeSessionID     string `json:"employee_session_id"`
	RuntimeSessionID      string `json:"runtime_session_id"`
	RuntimeStreamID       string `json:"runtime_stream_id"`
	RuntimeResponseID     string `json:"runtime_response_stream_id"`
	RuntimeTraceID        string `json:"runtime_trace_id"`
	RuntimeTurnID         string `json:"runtime_turn_id"`
	StreamURL             string `json:"stream_url"`
	ResponseStreamURL     string `json:"response_stream_url"`
	Created               bool   `json:"created"`
	Source                string `json:"source"`
	SourceResourceKey     string `json:"source_resource_key"`
	RuntimeConversationID string `json:"runtime_conversation_id"`
}

// SendSessionMessage handles POST /v1/employees/{id}/sessions/messages.
// @Summary Send a web session message
// @Description Creates a web-backed employee session when session_id is omitted, or sends a new turn to an existing web session. Returns a backend SSE URL for the response stream.
// @Tags employees
// @Accept json
// @Produce json
// @Param id path string true "Employee agent ID"
// @Param body body sendEmployeeSessionMessageRequest true "Message payload"
// @Success 202 {object} sendEmployeeSessionMessageResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sessions/messages [post]
func (h *EmployeeHandler) SendSessionMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	employeeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}
	if _, ok := h.loadActiveEmployee(ctx, org.ID, employeeID, w); !ok {
		return
	}

	var req sendEmployeeSessionMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	text := strings.TrimSpace(firstNonEmptyString(req.Text, req.Message))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}

	session, conversationID, created, err := h.webSessionForMessage(ctx, org.ID, employeeID, req.SessionID, text)
	if err != nil {
		writeEmployeeSessionMessageError(w, err)
		return
	}
	client, err := h.runtimeClientForSession(ctx, session)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to employee runtime"})
		return
	}
	delivery, err := client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:            text,
		ConversationID:  conversationID,
		User:            firstNonEmptyString(middleware.UserID(ctx), "web"),
		UserDisplayName: webUserDisplayName(ctx),
		Raw: map[string]any{
			"source":              webSessionSource,
			"employee_session_id": session.ID.String(),
			"channel_id":          webSessionSource,
			"thread_id":           session.SourceResourceKey,
		},
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to send message to employee runtime"})
		return
	}
	streamURL, err := h.signedWebStreamURL(employeeID, session.ID, delivery.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create stream URL"})
		return
	}
	responseStreamURL, err := h.signedWebStreamURL(employeeID, session.ID, delivery.ResponseStreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create stream URL"})
		return
	}

	writeJSON(w, http.StatusAccepted, sendEmployeeSessionMessageResponse{
		EmployeeSessionID:     session.ID.String(),
		RuntimeSessionID:      delivery.SessionID,
		RuntimeStreamID:       delivery.StreamID,
		RuntimeResponseID:     delivery.ResponseStreamID,
		RuntimeTraceID:        delivery.TraceID,
		RuntimeTurnID:         delivery.TurnID,
		StreamURL:             streamURL,
		ResponseStreamURL:     responseStreamURL,
		Created:               created,
		Source:                session.Source,
		SourceResourceKey:     session.SourceResourceKey,
		RuntimeConversationID: session.RuntimeConversationID,
	})
}

// StreamSession handles GET /v1/employees/{id}/sessions/{sessionID}/streams/{streamID}.
// @Summary Stream a web session turn
// @Description Proxies a signed runtime SSE stream for an authenticated web session.
// @Tags employees
// @Produce text/event-stream
// @Param id path string true "Employee agent ID"
// @Param sessionID path string true "Employee session ID"
// @Param streamID path string true "Runtime HTTP stream ID"
// @Param token query string true "Signed stream token"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sessions/{sessionID}/streams/{streamID} [get]
func (h *EmployeeHandler) StreamSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	employeeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid stream id"})
		return
	}
	if !h.verifyWebStreamToken(sessionID, streamID, r.URL.Query().Get("token")) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid stream token"})
		return
	}

	session, ok := h.loadWebSession(ctx, org.ID, employeeID, sessionID, w)
	if !ok {
		return
	}
	client, err := h.runtimeClientForSession(ctx, session)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to employee runtime"})
		return
	}
	resp, err := client.StreamHTTP(ctx, "/gateway/http/streams/"+streamID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to open employee runtime stream"})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.Copy(flushWriter{ResponseWriter: w}, resp.Body)
}
