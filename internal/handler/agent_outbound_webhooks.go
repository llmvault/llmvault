package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
)

type AgentOutboundWebhookHandler struct {
	db            *gorm.DB
	encKey        *crypto.SymmetricKey
	enqueuer      enqueue.TaskEnqueuer
	writer        *AgentEventWriter
	preloadCache  precontext.Cache
	now           func() time.Time
	maxBytes      int64
	maxBatchBytes int64
}

type agentOutboundEvent struct {
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	At        time.Time       `json:"at"`
}

func NewAgentOutboundWebhookHandler(db *gorm.DB, encKey *crypto.SymmetricKey, enqueuer enqueue.TaskEnqueuer, writers ...*AgentEventWriter) *AgentOutboundWebhookHandler {
	h := &AgentOutboundWebhookHandler{
		db:            db,
		encKey:        encKey,
		enqueuer:      enqueuer,
		now:           time.Now,
		maxBytes:      512 * 1024,
		maxBatchBytes: 10 * 1024 * 1024,
	}
	if len(writers) > 0 {
		h.writer = writers[0]
	}
	return h
}

func (h *AgentOutboundWebhookHandler) SetPreContextCache(cache precontext.Cache) {
	h.preloadCache = cache
	if h.writer != nil {
		h.writer.SetAfterWrite(func(ctx context.Context, events []model.SessionEvent) {
			for _, event := range events {
				precontext.InvalidateSessions(ctx, cache, event.OrgID, event.AgentID)
			}
		})
	}
}

func (h *AgentOutboundWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sb, ok := h.loadSandbox(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBytes))
	if err != nil {
		captureAgentWebhookIngest(ctx, "read_body", sb, nil, "", "", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if !h.verifySignature(ctx, sb, body, r.Header.Get("X-Hivy-Signature")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	var event agentOutboundEvent
	if err := json.Unmarshal(body, &event); err != nil {
		captureAgentWebhookIngest(ctx, "decode_webhook_payload", sb, nil, "", "", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}
	if event.At.IsZero() {
		event.At = h.now().UTC()
	}
	if err := h.storeAndMaybeEnqueue(ctx, sb, &event); err != nil {
		// A revenue- or reply-bearing record could not be durably persisted.
		// Return 5xx so the runtime outbox redelivers; the generation insert is
		// idempotent (deterministic ID), so the retry will not double-bill.
		captureAgentWebhookIngest(ctx, "store_outbound_event", sb, &event, "", "", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist event"})
		return
	}
	if err := h.db.WithContext(ctx).Model(sb).Update("last_active_at", h.now()).Error; err != nil {
		captureAgentWebhookIngest(ctx, "update_last_active", sb, &event, "", "", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AgentOutboundWebhookHandler) loadSandbox(w http.ResponseWriter, r *http.Request) (*model.Sandbox, bool) {
	sandboxID, err := uuid.Parse(chi.URLParam(r, "sandboxID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return nil, false
	}
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return nil, false
		}
		captureAgentWebhookIngest(r.Context(), "load_sandbox", &model.Sandbox{ID: sandboxID}, nil, "", "", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil, false
	}
	return &sb, true
}

func (h *AgentOutboundWebhookHandler) verifySignature(ctx context.Context, sb *model.Sandbox, body []byte, signature string) bool {
	if h.encKey == nil {
		// Fail closed: without the encryption key we can't recover the runtime
		// secret to verify the HMAC, so anyone could forge events for any sandbox.
		logging.FromContext(ctx).ErrorContext(ctx, "agent outbound webhook: rejecting request, no encryption key configured for signature verification",
			"sandbox_id", sb.ID)
		return false
	}
	secret, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "agent outbound webhook: failed to decrypt runtime secret",
			"sandbox_id", sb.ID, "error", err)
		captureAgentWebhookIngest(ctx, "decrypt_runtime_secret", sb, nil, "", "", err)
		return false
	}
	signature = strings.TrimSpace(strings.TrimPrefix(signature, "sha256="))
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
