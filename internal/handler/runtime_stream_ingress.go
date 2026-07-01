package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/tasks"
)

type RuntimeStreamIngressHandler struct {
	db       *gorm.DB
	encKey   *crypto.SymmetricKey
	store    *runtimestream.Store
	enqueuer enqueue.TaskEnqueuer
	upgrader websocket.Upgrader
}

func NewRuntimeStreamIngressHandler(db *gorm.DB, encKey *crypto.SymmetricKey, store *runtimestream.Store, enqueuer enqueue.TaskEnqueuer) *RuntimeStreamIngressHandler {
	return &RuntimeStreamIngressHandler{
		db:       db,
		encKey:   encKey,
		store:    store,
		enqueuer: enqueuer,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

type runtimeIngressFrame struct {
	Type   string                `json:"type,omitempty"`
	Event  *runtimestream.Event  `json:"event,omitempty"`
	Events []runtimestream.Event `json:"events,omitempty"`
}

type runtimeIngressAck struct {
	Type    string                    `json:"type"`
	AckSeq  int64                     `json:"ack_seq"`
	Results []runtimeIngressAckResult `json:"results"`
}

type runtimeIngressAckResult struct {
	SessionID   string `json:"session_id,omitempty"`
	RuntimeSeq  int64  `json:"runtime_seq"`
	Status      string `json:"status"`
	ExpectedSeq int64  `json:"expected_seq,omitempty"`
	StreamKey   string `json:"stream_key,omitempty"`
	StreamID    string `json:"stream_id,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (h *RuntimeStreamIngressHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	h.handleWS(w, r, "")
}

func (h *RuntimeStreamIngressHandler) HandleSessionWS(w http.ResponseWriter, r *http.Request) {
	h.handleWS(w, r, strings.TrimSpace(chi.URLParam(r, "sessionID")))
}

func (h *RuntimeStreamIngressHandler) handleWS(w http.ResponseWriter, r *http.Request, routeSessionID string) {
	if h == nil || h.db == nil || h.store == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "runtime stream ingress is not configured"})
		return
	}
	sb, ok := h.loadRuntimeIngressSandbox(w, r)
	if !ok {
		return
	}
	if !h.verifyRuntimeBearer(r.Context(), sb, r.Header.Get("Authorization")) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid runtime stream credentials"})
		return
	}
	var boundSession *model.Session
	if routeSessionID != "" {
		session, ok := h.loadRuntimeIngressSession(w, r, sb, routeSessionID)
		if !ok {
			return
		}
		boundSession = session
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "runtime stream ingress: upgrade websocket", "sandbox_id", sb.ID, "error", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(10 * 1024 * 1024)

	// The runtime has (re)connected, so its sandbox is awake. If auto-sleep had
	// marked it 'stopped', flip it back to 'running'.
	if sb.Status == string(sandbox.StatusStopped) {
		if err := tasks.EnqueueSandboxMarkRunning(r.Context(), h.enqueuer, sb.ID); err != nil {
			logging.FromContext(r.Context()).WarnContext(r.Context(), "runtime stream ingress: enqueue mark-running", "sandbox_id", sb.ID, "error", err)
		}
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				logging.FromContext(r.Context()).DebugContext(r.Context(), "runtime stream ingress: websocket read stopped", "sandbox_id", sb.ID, "error", err)
			}
			return
		}
		events, err := decodeRuntimeIngressEvents(raw)
		if err != nil {
			_ = writeRuntimeIngressJSON(conn, runtimeIngressAck{
				Type: "nack",
				Results: []runtimeIngressAckResult{{
					SessionID: routeSessionID,
					Status:    "invalid",
					Error:     err.Error(),
				}},
			})
			continue
		}
		ack, closeAfterWrite := h.acceptRuntimeEvents(r.Context(), sb, boundSession, events)
		if err := writeRuntimeIngressJSON(conn, ack); err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "runtime stream ingress: write ack", "sandbox_id", sb.ID, "error", err)
			return
		}
		if closeAfterWrite {
			return
		}
	}
}

func decodeRuntimeIngressEvents(raw []byte) ([]runtimestream.Event, error) {
	var frame runtimeIngressFrame
	if err := json.Unmarshal(raw, &frame); err == nil && (frame.Event != nil || len(frame.Events) > 0) {
		events := make([]runtimestream.Event, 0, len(frame.Events)+1)
		if frame.Event != nil {
			events = append(events, *frame.Event)
		}
		events = append(events, frame.Events...)
		return events, nil
	}
	var event runtimestream.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("decode runtime event frame: %w", err)
	}
	event.Normalize()
	return []runtimestream.Event{event}, nil
}

func (h *RuntimeStreamIngressHandler) acceptRuntimeEvents(ctx context.Context, sb *model.Sandbox, boundSession *model.Session, events []runtimestream.Event) (runtimeIngressAck, bool) {
	ack := runtimeIngressAck{
		Type:    "ack",
		Results: make([]runtimeIngressAckResult, 0, len(events)),
	}
	closeAfterWrite := false
	for _, event := range events {
		accepted, err := h.enrichRuntimeEvent(ctx, sb, boundSession, &event)
		if err != nil {
			ack.Type = "nack"
			ack.Results = append(ack.Results, runtimeIngressAckResult{
				SessionID:  firstNonEmptyString(event.SessionID, routeBoundSessionID(boundSession)),
				RuntimeSeq: event.RuntimeSeq,
				Status:     "invalid",
				Error:      err.Error(),
			})
			closeAfterWrite = true
			break
		}
		result, err := h.store.Append(ctx, accepted)
		if err != nil {
			ack.Type = "nack"
			ack.Results = append(ack.Results, runtimeIngressAckResult{
				SessionID:  accepted.SessionID,
				RuntimeSeq: accepted.RuntimeSeq,
				Status:     "error",
				Error:      err.Error(),
			})
			closeAfterWrite = true
			break
		}
		ack.Results = append(ack.Results, runtimeIngressAckResult{
			SessionID:   accepted.SessionID,
			RuntimeSeq:  accepted.RuntimeSeq,
			Status:      result.Status,
			ExpectedSeq: result.ExpectedSeq,
			StreamKey:   result.StreamKey,
			StreamID:    result.StreamID,
			Error:       result.Error,
		})
		if result.Status == runtimestream.AppendGap || result.Status == runtimestream.AppendConflict {
			ack.Type = "nack"
			closeAfterWrite = true
			break
		}
		if accepted.RuntimeSeq > ack.AckSeq {
			ack.AckSeq = accepted.RuntimeSeq
		}
	}
	return ack, closeAfterWrite
}

func (h *RuntimeStreamIngressHandler) enrichRuntimeEvent(ctx context.Context, sb *model.Sandbox, boundSession *model.Session, event *runtimestream.Event) (runtimestream.Event, error) {
	if sb == nil || sb.OrgID == nil || sb.AgentID == nil {
		return runtimestream.Event{}, fmt.Errorf("sandbox is not attached to an org and agent")
	}
	event.Normalize()
	if event.SessionID == "" && boundSession != nil {
		event.SessionID = boundSession.ID.String()
	}
	if event.SandboxID == "" {
		event.SandboxID = sb.ID.String()
	}
	if event.SandboxID != sb.ID.String() {
		return runtimestream.Event{}, fmt.Errorf("event sandbox_id does not match websocket sandbox")
	}
	sessionID, err := uuid.Parse(event.SessionID)
	if err != nil {
		return runtimestream.Event{}, fmt.Errorf("invalid session_id: %w", err)
	}
	var session model.Session
	if boundSession != nil {
		if boundSession.ID != sessionID {
			return runtimestream.Event{}, fmt.Errorf("event session_id does not match websocket session")
		}
		session = *boundSession
	} else {
		err = h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, *sb.OrgID, *sb.AgentID).
			First(&session).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return runtimestream.Event{}, fmt.Errorf("session not found for sandbox")
			}
			return runtimestream.Event{}, fmt.Errorf("load session: %w", err)
		}
	}
	if session.SandboxID != nil && *session.SandboxID != sb.ID {
		return runtimestream.Event{}, fmt.Errorf("session is attached to a different sandbox")
	}
	event.OrgID = session.OrgID.String()
	event.AgentID = session.AgentID.String()
	event.SessionID = session.ID.String()
	if event.SandboxID == "" && session.SandboxID != nil {
		event.SandboxID = session.SandboxID.String()
	}
	if event.Source == "" {
		event.Source = "runtime"
	}
	if event.EventID == "" {
		event.EventID = fmt.Sprintf("runtime-%s-%d", event.SessionID, event.RuntimeSeq)
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if err := event.Validate(); err != nil {
		return runtimestream.Event{}, err
	}
	return *event, nil
}
