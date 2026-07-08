package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

// IncomingWebhookHandler receives webhook events directly from external
// providers that require manual webhook URL configuration (e.g. Railway).
// Unlike the Nango webhook path, these arrive without an intermediary envelope.
type IncomingWebhookHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

// NewIncomingWebhookHandler creates an incoming webhook handler.
func NewIncomingWebhookHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *IncomingWebhookHandler {
	return &IncomingWebhookHandler{db: db, enqueuer: enqueuer}
}

// Handle processes POST /incoming/webhooks/{provider}/{connectionID}. The endpoint
// is unauthenticated — the unguessable connectionID acts as a bearer token.
// @Summary Receive incoming webhook from external provider
// @Description Receives webhook events directly from providers that require manual webhook URL configuration (e.g. Railway). The connection UUID in the URL identifies the org and connection.
// @Tags webhooks
// @Accept json
// @Produce json
// @Param provider path string true "Provider name (e.g. railway)"
// @Param connectionID path string true "Connection UUID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /incoming/webhooks/{provider}/{connectionID} [post]
func (h *IncomingWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	connectionIDStr := chi.URLParam(r, "connectionID")

	connectionID, err := uuid.Parse(connectionIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection ID"})
		return
	}

	cat := catalog.Global()
	providerTriggers, hasTriggers := cat.GetProviderTriggers(provider)
	if !hasTriggers {
		providerTriggers, hasTriggers = cat.GetProviderTriggersForVariant(provider)
	}
	if !hasTriggers || providerTriggers.WebhookConfig == nil || !providerTriggers.WebhookConfig.WebhookURLRequired {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "provider not configured for direct webhooks"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "incoming webhook: failed to read body",
			"provider", provider,
			"error", err,
		)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "failed to read body"})
		return
	}

	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "empty body"})
		return
	}

	var connection model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND revoked_at IS NULL", connectionID).
		First(&connection).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
		return
	}

	if connection.Integration.DeletedAt != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	// Beyond the unguessable connection UUID, require a shared secret / native
	// signature when one is configured on the connection. Direct providers
	// (e.g. Railway) do not sign, so operators may configure a shared secret
	// (connection.Meta["webhook_secret"]) delivered as X-Hivy-Webhook-Secret;
	// GitHub-style senders may instead present X-Hub-Signature-256. If a secret
	// is configured, an unsigned/mismatched request is rejected. If none is
	// configured we proceed (UUID-only) but warn, so the gap is observable.
	if secret := connectionWebhookSecret(connection); secret != "" {
		ghSig := r.Header.Get("X-Hub-Signature-256")
		sharedSig := r.Header.Get("X-Hivy-Webhook-Secret")
		ok := false
		switch {
		case ghSig != "":
			ok = verifyGitHubSignature256(body, secret, ghSig)
		case sharedSig != "":
			ok = verifyWebhookSharedSecret(secret, sharedSig)
		}
		if !ok {
			logging.FromContext(r.Context()).WarnContext(r.Context(), "incoming webhook rejected: signature/secret invalid",
				"provider", provider, "connection_id", connectionID.String())
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid signature"})
			return
		}
	} else {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "incoming webhook has no configured secret; relying on connection UUID only",
			"provider", provider, "connection_id", connectionID.String())
	}

	eventType, eventAction := inferDirectWebhookEvent(provider, body)
	if eventType == "" {

		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "unknown event type"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	deliveryID := connectionID.String() + ":" + uuid.New().String()
	task, opts, err := tasks.NewAgentTriggerDispatchTask(tasks.AgentTriggerDispatchPayload{
		Provider:     provider,
		EventType:    eventType,
		EventAction:  eventAction,
		DeliveryID:   deliveryID,
		OrgID:        connection.OrgID,
		ConnectionID: connectionID,
		PayloadJSON:  body,
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "incoming webhook: failed to build dispatch task",
			"provider", provider,
			"error", err,
		)
		logging.CaptureWithFields(r.Context(), fmt.Errorf("incoming webhook: failed to build dispatch task: %w", err), map[string]any{
			"org_id":      connection.OrgID.String(),
			"delivery_id": deliveryID,
			"event_key":   eventKeyForHandler(eventType, eventAction),
		})
		return
	}

	if _, err := h.enqueuer.Enqueue(task, opts...); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "incoming webhook: failed to enqueue dispatch task",
			"provider", provider,
			"error", err,
		)
		logging.CaptureWithFields(r.Context(), fmt.Errorf("incoming webhook: failed to enqueue dispatch task: %w", err), map[string]any{
			"org_id":      connection.OrgID.String(),
			"delivery_id": deliveryID,
			"event_key":   eventKeyForHandler(eventType, eventAction),
		})
		return
	}
}

// connectionWebhookSecret reads an optional per-connection webhook shared
// secret from the connection metadata. Empty when unset.
func connectionWebhookSecret(conn model.Connection) string {
	if conn.Meta == nil {
		return ""
	}
	if v, ok := conn.Meta["webhook_secret"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func eventKeyForHandler(eventType, eventAction string) string {
	if eventAction == "" {
		return eventType
	}
	return eventType + "." + eventAction
}

// inferDirectWebhookEvent extracts the event type and action from a raw
// webhook payload for providers that send webhooks directly (not via Nango).
func inferDirectWebhookEvent(provider string, body []byte) (eventType, eventAction string) {
	switch {
	case provider == "railway" || strings.HasPrefix(provider, "railway"):
		return inferRailwayEvent(body)
	}
	return "", ""
}

// inferRailwayEvent extracts the event type from a Railway webhook payload
// ({"type":"Deployment.failed"}); the type maps directly to trigger keys.
func inferRailwayEvent(body []byte) (eventType, eventAction string) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || probe.Type == "" {
		return "", ""
	}
	return probe.Type, ""
}
