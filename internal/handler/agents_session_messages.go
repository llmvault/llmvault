package handler

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/middleware"
)

const webSessionSource = "web"

type sendAgentSessionMessageRequest struct {
	Text      string `json:"text"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

type sendAgentSessionMessageResponse struct {
	AgentSessionID          string `json:"agent_session_id"`
	RuntimeSessionID        string `json:"runtime_session_id"`
	RuntimeStreamID         string `json:"runtime_stream_id"`
	RuntimeResponseID       string `json:"runtime_response_stream_id"`
	RuntimeTraceID          string `json:"runtime_trace_id"`
	RuntimeTurnID           string `json:"runtime_turn_id"`
	StreamURL               string `json:"stream_url"`
	ResponseStreamURL       string `json:"response_stream_url"`
	DirectStreamURL         string `json:"direct_stream_url,omitempty"`
	DirectResponseStreamURL string `json:"direct_response_stream_url,omitempty"`
	Created                 bool   `json:"created"`
	Source                  string `json:"source"`
	SourceResourceKey       string `json:"source_resource_key"`
	RuntimeConversationID   string `json:"runtime_conversation_id"`
}

// SendSessionMessage handles POST /v1/agents/{id}/sessions/messages.
// @Summary Send a web session message
// @Description Creates a web-backed agent session when session_id is omitted, or sends a new turn to an existing web session. Returns a backend SSE URL for the response stream.
// @Tags agents
// @Accept json
// @Produce json
// @Param id path string true "Agent agent ID"
// @Param body body sendAgentSessionMessageRequest true "Message payload"
// @Success 202 {object} sendAgentSessionMessageResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sessions/messages [post]
func (h *AgentHandler) SendSessionMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	if _, ok := h.loadActiveAgent(ctx, org.ID, agentID, w); !ok {
		return
	}

	var req sendAgentSessionMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	text := strings.TrimSpace(firstNonEmptyString(req.Text, req.Message))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}

	session, conversationID, created, err := h.webSessionForMessage(ctx, org.ID, agentID, req.SessionID, text)
	if err != nil {
		writeAgentSessionMessageError(w, err)
		return
	}
	client, sandbox, err := h.runtimeClientAndSandboxForSession(ctx, session)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to agent runtime"})
		return
	}
	delivery, err := client.PostHTTPMessage(ctx, agentruntime.HTTPMessageRequest{
		Text:            text,
		ConversationID:  conversationID,
		User:            firstNonEmptyString(middleware.UserID(ctx), "web"),
		UserDisplayName: webUserDisplayName(ctx),
		Raw: map[string]any{
			"source":           webSessionSource,
			"agent_session_id": session.ID.String(),
			"channel_id":       webSessionSource,
			"thread_id":        session.SourceResourceKey,
		},
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to send message to agent runtime"})
		return
	}
	streamURL, err := h.signedWebStreamURL(agentID, session.ID, delivery.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create stream URL"})
		return
	}
	responseStreamURL, err := h.signedWebStreamURL(agentID, session.ID, delivery.ResponseStreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create stream URL"})
		return
	}
	directStreamURL, directResponseStreamURL := h.directWebStreamURLs(sandbox, delivery)

	writeJSON(w, http.StatusAccepted, sendAgentSessionMessageResponse{
		AgentSessionID:          session.ID.String(),
		RuntimeSessionID:        delivery.SessionID,
		RuntimeStreamID:         delivery.StreamID,
		RuntimeResponseID:       delivery.ResponseStreamID,
		RuntimeTraceID:          delivery.TraceID,
		RuntimeTurnID:           delivery.TurnID,
		StreamURL:               streamURL,
		ResponseStreamURL:       responseStreamURL,
		DirectStreamURL:         directStreamURL,
		DirectResponseStreamURL: directResponseStreamURL,
		Created:                 created,
		Source:                  session.Source,
		SourceResourceKey:       session.SourceResourceKey,
		RuntimeConversationID:   session.RuntimeConversationID,
	})
}

// StreamSession handles GET /v1/agents/{id}/sessions/{sessionID}/streams/{streamID}.
// @Summary Stream a web session turn
// @Description Proxies a signed runtime SSE stream for an authenticated web session.
// @Tags agents
// @Produce text/event-stream
// @Param id path string true "Agent agent ID"
// @Param sessionID path string true "Agent session ID"
// @Param streamID path string true "Runtime HTTP stream ID"
// @Param token query string true "Signed stream token"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/sessions/{sessionID}/streams/{streamID} [get]
func (h *AgentHandler) StreamSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
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
	if !h.verifyWebStreamToken(sessionID, streamID, extractWebStreamToken(r)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid stream token"})
		return
	}

	session, ok := h.loadWebSession(ctx, org.ID, agentID, sessionID, w)
	if !ok {
		return
	}
	client, err := h.runtimeClientForSession(ctx, session)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to connect to agent runtime"})
		return
	}
	resp, err := client.StreamHTTP(ctx, "/gateway/http/streams/"+streamID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to open agent runtime stream"})
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
	_ = copySSEStream(w, resp.Body)
}

func copySSEStream(w http.ResponseWriter, r io.Reader) error {
	reader := bufio.NewReader(r)
	flusher, _ := w.(http.Flusher)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			if _, writeErr := io.WriteString(w, line); writeErr != nil {
				return writeErr
			}
			if isSSEEventBoundary(line) && flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				if flusher != nil {
					flusher.Flush()
				}
				return nil
			}
			return err
		}
	}
}

func isSSEEventBoundary(line string) bool {
	return strings.TrimRight(line, "\r\n") == ""
}
