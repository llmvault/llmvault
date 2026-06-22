package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/runtimestream"
)

const sessionStreamBatchSize = 500

func (h *SessionHandler) Stream(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.runtimeStreamStore == nil || h.runtimeStreamStore.Redis() == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session stream is not configured"})
		return
	}
	session, _, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}
	afterSeq, err := parseAfterSeq(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	ctx := r.Context()
	sessionID := session.ID.String()
	pubsub := h.runtimeStreamStore.Redis().Subscribe(ctx, runtimestream.LiveChannel(sessionID))
	defer pubsub.Close()
	if _, err := pubsub.Receive(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "failed to subscribe to session stream"})
		return
	}
	liveCh := pubsub.Channel(redis.WithChannelSize(1024))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	cursor, err := h.emitCommittedSince(ctx, rc, w, session.ID, afterSeq)
	if err != nil {
		_ = writeSessionSSE(rc, w, "session.control", "", map[string]any{
			"type":  "resync",
			"error": "failed to load committed events",
		})
		return
	}
	if err := h.drainBufferedLive(rc, w, liveCh, sessionID, &cursor); err != nil {
		return
	}

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-liveCh:
			if !ok {
				_ = writeSessionSSE(rc, w, "session.control", "", map[string]any{
					"type": "resync",
				})
				return
			}
			if err := h.emitLiveMessage(rc, w, msg.Payload, sessionID, &cursor); err != nil {
				return
			}
		case <-keepalive.C:
			if err := writeSessionSSE(rc, w, "session.control", "", map[string]any{
				"type":      "keepalive",
				"cursor":    cursor,
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			}); err != nil {
				return
			}
		case <-poll.C:
			next, err := h.emitCommittedSince(ctx, rc, w, session.ID, cursor)
			if err != nil {
				_ = writeSessionSSE(rc, w, "session.control", "", map[string]any{
					"type":  "resync",
					"error": "failed to poll committed events",
				})
				return
			}
			cursor = next
		}
	}
}

func parseAfterSeq(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("after_seq")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	if raw == "" {
		return 0, nil
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq < 0 {
		return 0, fmt.Errorf("after_seq must be a non-negative integer")
	}
	return seq, nil
}

func (h *SessionHandler) drainBufferedLive(rc *http.ResponseController, w http.ResponseWriter, liveCh <-chan *redis.Message, sessionID string, cursor *int64) error {
	for {
		select {
		case msg, ok := <-liveCh:
			if !ok {
				return nil
			}
			if err := h.emitLiveMessage(rc, w, msg.Payload, sessionID, cursor); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (h *SessionHandler) emitCommittedSince(ctx context.Context, rc *http.ResponseController, w http.ResponseWriter, sessionID uuid.UUID, after int64) (int64, error) {
	cursor := after
	for {
		var events []model.SessionEvent
		if err := h.db.WithContext(ctx).
			Where("session_id = ? AND sequence_number > ?", sessionID, cursor).
			Order("sequence_number ASC, created_at ASC, id ASC").
			Limit(sessionStreamBatchSize).
			Find(&events).Error; err != nil {
			return cursor, err
		}
		for _, event := range events {
			view := runtimestream.SessionEventToView(event)
			if err := writeSessionSSE(rc, w, "session.event", strconv.FormatInt(event.SequenceNumber, 10), view); err != nil {
				return cursor, err
			}
			if event.SequenceNumber > cursor {
				cursor = event.SequenceNumber
			}
		}
		if len(events) < sessionStreamBatchSize {
			return cursor, nil
		}
	}
}

func (h *SessionHandler) emitLiveMessage(rc *http.ResponseController, w http.ResponseWriter, raw string, sessionID string, cursor *int64) error {
	var msg runtimestream.LiveMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return nil
	}
	if msg.SessionID != sessionID {
		return nil
	}
	switch msg.Kind {
	case runtimestream.LiveKindRuntime:
		if msg.Event == nil {
			return nil
		}
		return writeSessionSSE(rc, w, "session.preview", "", msg.Event)
	case runtimestream.LiveKindCommitted:
		if msg.Committed == nil {
			return nil
		}
		if msg.Committed.SequenceNumber <= *cursor {
			return nil
		}
		if err := writeSessionSSE(rc, w, "session.event", strconv.FormatInt(msg.Committed.SequenceNumber, 10), msg.Committed); err != nil {
			return err
		}
		*cursor = msg.Committed.SequenceNumber
		return nil
	default:
		return nil
	}
}

func writeSessionSSE(rc *http.ResponseController, w http.ResponseWriter, eventName, id string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_ = rc.SetWriteDeadline(time.Now().Add(20 * time.Second))
	if eventName != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return rc.Flush()
}
