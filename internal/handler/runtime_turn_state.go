package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const hivySignatureHeader = "X-Hivy-Signature"

type runtimeTurnStateEvent struct {
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	At        time.Time      `json:"at"`
}

// HandleTurnState receives fast best-effort runtime turn state callbacks.
// Durable runtime ingestion remains the reconciliation source; this endpoint
// only narrows the visible active/idle lag while callbacks arrive in order.
func (h *RuntimeStreamIngressHandler) HandleTurnState(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.db == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "runtime turn state is not configured"})
		return
	}
	sb, ok := h.loadRuntimeIngressSandbox(w, r)
	if !ok {
		return
	}
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime turn state auth is not configured"})
		return
	}
	secret, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "runtime turn state: failed to decrypt runtime secret", "sandbox_id", sb.ID, "error", err)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid runtime credentials"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "failed to read body"})
		return
	}
	if !verifyHivyWebhookSignature(secret, body, r.Header.Get(hivySignatureHeader)) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid signature"})
		return
	}

	var event runtimeTurnStateEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid turn state payload"})
		return
	}
	result, err := h.applyRuntimeTurnState(r.Context(), sb, event)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"updated": result.RowsAffected > 0,
	})
}

func (h *RuntimeStreamIngressHandler) applyRuntimeTurnState(ctx context.Context, sb *model.Sandbox, event runtimeTurnStateEvent) (*gorm.DB, error) {
	if sb == nil || sb.OrgID == nil || sb.AgentID == nil {
		return nil, fmt.Errorf("sandbox is not attached to an org and agent")
	}
	sessionID, err := uuid.Parse(payloadString(event.Payload, "session_id"))
	if err != nil {
		return nil, fmt.Errorf("invalid session_id")
	}
	turnID := strings.TrimSpace(payloadString(event.Payload, "turn_id"))
	if turnID == "" {
		return nil, fmt.Errorf("turn_id is required")
	}
	streamID := strings.TrimSpace(payloadString(event.Payload, "stream_id"))
	occurredAt := payloadTime(event.Payload, "occurred_at")
	if occurredAt.IsZero() {
		occurredAt = event.At
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	switch event.EventType {
	case "turn_started":
		updates := map[string]any{
			"agent_turn_id":           turnID,
			"agent_stream_id":         defaultString(streamID, turnID),
			"agent_turn_last_outcome": "",
			"agent_turn_started_at":   occurredAt,
			"updated_at":              occurredAt,
		}
		return h.db.WithContext(ctx).Model(&model.Session{}).
			Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, *sb.OrgID, *sb.AgentID).
			Where("sandbox_id IS NULL OR sandbox_id = ?", sb.ID).
			Where("agent_turn_status = ? AND (agent_turn_id = '' OR agent_turn_id = ?)", model.SessionAgentTurnActive, turnID).
			Updates(updates), nil
	case "turn_completed", "turn_failed", "turn_interrupted":
		outcome := model.SessionAgentTurnOutcomeDone
		if event.EventType == "turn_failed" {
			outcome = model.SessionAgentTurnOutcomeFailed
		}
		if event.EventType == "turn_interrupted" {
			outcome = model.SessionAgentTurnOutcomeStopped
		}
		updates := map[string]any{
			"agent_turn_status":       model.SessionAgentTurnIdle,
			"agent_turn_id":           "",
			"agent_stream_id":         "",
			"agent_turn_started_at":   nil,
			"agent_turn_last_outcome": outcome,
			"updated_at":              occurredAt,
		}
		return h.db.WithContext(ctx).Model(&model.Session{}).
			Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, *sb.OrgID, *sb.AgentID).
			Where("sandbox_id IS NULL OR sandbox_id = ?", sb.ID).
			Where("agent_turn_status = ? AND agent_turn_id = ?", model.SessionAgentTurnActive, turnID).
			Updates(updates), nil
	default:
		return nil, fmt.Errorf("unsupported turn state event")
	}
}

func verifyHivyWebhookSignature(secret string, body []byte, rawSignature string) bool {
	signature := strings.TrimSpace(rawSignature)
	signature = strings.TrimPrefix(signature, "sha256=")
	if secret == "" || signature == "" {
		return false
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func payloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func payloadTime(payload map[string]any, key string) time.Time {
	value := payloadString(payload, key)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
