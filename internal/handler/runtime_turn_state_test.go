package handler_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

func TestRuntimeTurnStateStartedFillsActiveTurn(t *testing.T) {
	h, sb, sessionID, secret := newRuntimeTurnStateFixture(t)
	markSessionTurn(t, h, sessionID, model.SessionAgentTurnActive, "", "", "")

	rr := postRuntimeTurnState(t, h, sb.ID, secret, map[string]any{
		"event_type": "turn_started",
		"payload": map[string]any{
			"session_id":  sessionID.String(),
			"turn_id":     "turn-1",
			"stream_id":   "stream-1",
			"occurred_at": "2026-06-22T14:00:00Z",
		},
		"at": "2026-06-22T14:00:00Z",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("turn state status=%d body=%s", rr.Code, rr.Body.String())
	}

	session := loadSession(t, h, sessionID)
	if session.AgentTurnStatus != model.SessionAgentTurnActive {
		t.Fatalf("agent_turn_status=%q", session.AgentTurnStatus)
	}
	if session.AgentTurnID != "turn-1" || session.AgentStreamID != "stream-1" {
		t.Fatalf("turn identifiers = %q/%q", session.AgentTurnID, session.AgentStreamID)
	}
	if session.AgentTurnStartedAt == nil || !session.AgentTurnStartedAt.Equal(time.Date(2026, 6, 22, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("agent_turn_started_at=%v", session.AgentTurnStartedAt)
	}
}

func TestRuntimeTurnStateTerminalClearsMatchingActiveTurn(t *testing.T) {
	h, sb, sessionID, secret := newRuntimeTurnStateFixture(t)
	markSessionTurn(t, h, sessionID, model.SessionAgentTurnActive, "turn-1", "stream-1", "")

	rr := postRuntimeTurnState(t, h, sb.ID, secret, map[string]any{
		"event_type": "turn_completed",
		"payload": map[string]any{
			"session_id":  sessionID.String(),
			"turn_id":     "turn-1",
			"occurred_at": "2026-06-22T14:00:03Z",
		},
		"at": "2026-06-22T14:00:03Z",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("turn state status=%d body=%s", rr.Code, rr.Body.String())
	}

	session := loadSession(t, h, sessionID)
	if session.AgentTurnStatus != model.SessionAgentTurnIdle {
		t.Fatalf("agent_turn_status=%q", session.AgentTurnStatus)
	}
	if session.AgentTurnID != "" || session.AgentStreamID != "" || session.AgentTurnStartedAt != nil {
		t.Fatalf("turn state not cleared: id=%q stream=%q started=%v", session.AgentTurnID, session.AgentStreamID, session.AgentTurnStartedAt)
	}
	if session.AgentTurnLastOutcome != model.SessionAgentTurnOutcomeDone {
		t.Fatalf("last outcome=%q", session.AgentTurnLastOutcome)
	}
}

func TestRuntimeTurnStateStaleTerminalDoesNotClearNewerTurn(t *testing.T) {
	h, sb, sessionID, secret := newRuntimeTurnStateFixture(t)
	markSessionTurn(t, h, sessionID, model.SessionAgentTurnActive, "turn-new", "stream-new", "")

	rr := postRuntimeTurnState(t, h, sb.ID, secret, map[string]any{
		"event_type": "turn_failed",
		"payload": map[string]any{
			"session_id": sessionID.String(),
			"turn_id":    "turn-old",
		},
		"at": "2026-06-22T14:00:04Z",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("turn state status=%d body=%s", rr.Code, rr.Body.String())
	}

	session := loadSession(t, h, sessionID)
	if session.AgentTurnStatus != model.SessionAgentTurnActive || session.AgentTurnID != "turn-new" || session.AgentStreamID != "stream-new" {
		t.Fatalf("stale terminal changed turn state: status=%q id=%q stream=%q", session.AgentTurnStatus, session.AgentTurnID, session.AgentStreamID)
	}
}

func TestRuntimeTurnStateRejectsInvalidSignature(t *testing.T) {
	h, sb, sessionID, _ := newRuntimeTurnStateFixture(t)

	rr := postRuntimeTurnState(t, h, sb.ID, "wrong-secret", map[string]any{
		"event_type": "turn_started",
		"payload": map[string]any{
			"session_id": sessionID.String(),
			"turn_id":    "turn-1",
		},
		"at": "2026-06-22T14:00:00Z",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("turn state status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func newRuntimeTurnStateFixture(t *testing.T) (*sessionHarness, model.Sandbox, uuid.UUID, string) {
	t.Helper()
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Start callback state")
	sessionID, err := uuid.Parse(created.Session.ID)
	if err != nil {
		t.Fatalf("parse session id: %v", err)
	}
	secret := "runtime-state-secret-" + randSuffix()
	encSecret, err := sessionTestEncKey(t).EncryptString(secret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             "docker",
		ExternalID:             "runtime-turn-state-" + randSuffix(),
		RuntimeURL:             "http://127.0.0.1:7080",
		EncryptedRuntimeSecret: encSecret,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := h.db.Model(&model.Session{}).Where("id = ?", sessionID).Update("sandbox_id", sb.ID).Error; err != nil {
		t.Fatalf("attach sandbox: %v", err)
	}
	return h, sb, sessionID, secret
}

func markSessionTurn(t *testing.T, h *sessionHarness, sessionID uuid.UUID, status, turnID, streamID, outcome string) {
	t.Helper()
	updates := map[string]any{
		"agent_turn_status":       status,
		"agent_turn_id":           turnID,
		"agent_stream_id":         streamID,
		"agent_turn_started_at":   nil,
		"agent_turn_last_outcome": outcome,
	}
	if err := h.db.Model(&model.Session{}).Where("id = ?", sessionID).Updates(updates).Error; err != nil {
		t.Fatalf("mark session turn: %v", err)
	}
}

func postRuntimeTurnState(t *testing.T, h *sessionHarness, sandboxID uuid.UUID, secret string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/runtime-events/sandboxes/"+sandboxID.String()+"/turn-state", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hivy-Signature", "sha256="+runtimeTurnStateSignature(secret, payload))
	req = withChiURLParam(req, "sandboxID", sandboxID.String())

	rr := httptest.NewRecorder()
	handler.NewRuntimeStreamIngressHandler(h.db, sessionTestEncKey(t), nil, nil).HandleTurnState(rr, req)
	return rr
}

func runtimeTurnStateSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func loadSession(t *testing.T, h *sessionHarness, sessionID uuid.UUID) model.Session {
	t.Helper()
	var session model.Session
	if err := h.db.First(&session, "id = ?", sessionID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	return session
}
