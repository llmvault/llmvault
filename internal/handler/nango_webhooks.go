package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/tasks"
)

// NangoWebhookHandler receives webhook events forwarded by Nango.
type NangoWebhookHandler struct {
	db             *gorm.DB
	nangoSecret    string
	encKey         *crypto.SymmetricKey
	httpClient     *http.Client
	enqueuer       enqueue.TaskEnqueuer
	nangoClient    *nango.Client
	gatewayService *gateway.Service
}

func NewNangoWebhookHandler(db *gorm.DB, nangoSecret string, encKey *crypto.SymmetricKey, nangoClient *nango.Client, gatewayService *gateway.Service, enqueuer ...enqueue.TaskEnqueuer) *NangoWebhookHandler {
	h := &NangoWebhookHandler{
		db:             db,
		nangoSecret:    nangoSecret,
		encKey:         encKey,
		httpClient:     &http.Client{Timeout: 25 * time.Second},
		nangoClient:    nangoClient,
		gatewayService: gatewayService,
	}
	if len(enqueuer) > 0 {
		h.enqueuer = enqueuer[0]
	}
	if h.gatewayService != nil && h.enqueuer != nil {
		h.gatewayService.SetConnectionInboundAcceptedHook(h.enqueueGatewaySlackStatus)
	}
	return h
}

type nangoWebhook struct {
	From              string          `json:"from"`
	Type              string          `json:"type"`
	ConnectionID      string          `json:"connectionId"`
	ProviderConfigKey string          `json:"providerConfigKey"`
	Provider          string          `json:"provider,omitempty"`
	Operation         string          `json:"operation,omitempty"`
	Success           *bool           `json:"success,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
}

type webhookContext struct {
	orgID      uuid.UUID
	connection *model.Connection
}

// Handle processes POST /internal/webhooks/nango.
func (h *NangoWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango webhook: failed to read request body", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	signature := r.Header.Get("X-Nango-Hmac-Sha256")
	if signature == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing signature header"})
		return
	}
	if !verifyNangoSignature(body, h.nangoSecret, signature) {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "nango webhook: invalid signature")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	var wh nangoWebhook
	if err := json.Unmarshal(body, &wh); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango webhook: failed to parse payload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook payload"})
		return
	}

	wctx := h.identify(r.Context(), &wh)
	if wctx == nil {
		headers := make(map[string]string)
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}
		logging.FromContext(r.Context()).InfoContext(r.Context(), "nango_webhook_connection_not_found",
			"nango_connection_id", wh.ConnectionID,
			"provider_config_key", wh.ProviderConfigKey,
			"type", wh.Type,
			"from", wh.From,
			"payload", string(body),
			"headers", headers,
		)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if isSlackProvider(wctx.connection) && wh.Type == "forward" {
		slackFields := slackWebhookSentryFields("start", &wh, wctx.connection, wh.Payload)
		employee, err := ensureHivyEmployee(r.Context(), h.db, wctx.connection.OrgID)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "slack_webhook_failed_to_ensure_employee",
				"org_id", wctx.connection.OrgID.String(),
				"error", err,
			)
			slackFields["stage"] = "ensure_employee"
			logging.CaptureWithFields(r.Context(), fmt.Errorf("slack webhook: ensure employee: %w", err), slackFields)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
			return
		}

		if h.gatewayService == nil || h.nangoClient == nil || h.enqueuer == nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "slack_webhook_missing_dependencies")
			slackFields["stage"] = "dependencies"
			logging.CaptureWithFields(r.Context(), fmt.Errorf("slack webhook: missing gateway dependencies"), slackFields)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "slack gateway not configured"})
			return
		}

		providerKey := nangoProviderConfigKey(wctx.connection.Integration.UniqueKey)
		envelope := gateway.WebhookEnvelope{
			ConnectionID: wctx.connection.ID,
			OrgID:        wctx.connection.OrgID,
			EmployeeID:   employee.ID,
			Provider:     wctx.connection.Integration.Provider,
			ProviderKey:  providerKey,
			NangoConnID:  wctx.connection.NangoConnectionID,
			Headers:      normalizedHeaders(r.Header),
			Body:         wh.Payload,
		}

		result, err := h.gatewayService.ReceiveWebhookFromConnection(r.Context(), envelope)
		if err != nil {
			slackFields["stage"] = "gateway_receive"
			logging.CaptureWithFields(r.Context(), fmt.Errorf("slack webhook: receive: %w", err), slackFields)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if result == nil || result.Ignored || result.Duplicate {
			status := "ignored"
			reason := "nil_result"
			if result != nil {
				reason = result.IgnoreReason
				if result.Duplicate {
					status = "duplicate"
				}
			}
			logging.FromContext(r.Context()).InfoContext(r.Context(), "slack_webhook_skipped",
				"connection_id", envelope.ConnectionID.String(),
				"org_id", envelope.OrgID.String(),
				"employee_id", envelope.EmployeeID.String(),
				"status", status,
				"reason", reason,
			)
			writeJSON(w, http.StatusOK, map[string]string{"status": status})
			return
		}

		task, err := tasks.NewGatewaySlackTask(gatewaySlackPayload(envelope, result, wctx.connection, providerKey))
		if err != nil {
			slackFields["stage"] = "build_task"
			logging.CaptureWithFields(r.Context(), fmt.Errorf("slack webhook: build task: %w", err), slackFields)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build task"})
			return
		}

		if _, err := h.enqueuer.EnqueueContext(r.Context(), task); err != nil {
			slackFields["stage"] = "enqueue_task"
			logging.CaptureWithFields(r.Context(), fmt.Errorf("slack webhook: enqueue task: %w", err), slackFields)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue task"})
			return
		}

		logging.FromContext(r.Context()).InfoContext(r.Context(), "slack_webhook_dispatched",
			"connection_id", envelope.ConnectionID.String(),
			"org_id", envelope.OrgID.String(),
			"employee_id", envelope.EmployeeID.String(),
			"channel_id", result.Inbound.ChannelID,
			"thread_ts", result.Inbound.ThreadID,
		)

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if !isSlackProvider(wctx.connection) {
		logging.FromContext(r.Context()).InfoContext(r.Context(), "nango_webhook_skipped",
			"org_id", wctx.orgID.String(),
			"connection_id", wctx.connection.ID.String(),
			"provider", wctx.connection.Integration.Provider,
			"type", wh.Type,
		)
	}

	dispatchWebhookEvent(r.Context(), h.enqueuer, &wh, wctx)

	h.acknowledge(w)
}

func gatewaySlackPayload(envelope gateway.WebhookEnvelope, result *gateway.ReceiveConnectionResult, conn *model.Connection, providerKey string) tasks.GatewaySlackPayload {
	return tasks.GatewaySlackPayload{
		ConnectionID:   envelope.ConnectionID.String(),
		OrgID:          envelope.OrgID.String(),
		EmployeeID:     envelope.EmployeeID.String(),
		ChannelID:      result.Inbound.ChannelID,
		ThreadTS:       result.Inbound.ThreadID,
		TeamID:         slackInboundTeamID(result.Inbound.Raw),
		StreamURL:      firstNonEmpty(result.ResponseStreamURL, result.StreamURL),
		RuntimeURL:     result.RuntimeURL,
		RuntimeAPIKey:  result.RuntimeAPIKey,
		SessionID:      result.Session.ID.String(),
		RuntimeConvoID: result.RuntimeConversationID,
		TraceID:        result.TraceID,
		TurnID:         result.TurnID,
		SenderID:       result.Inbound.SenderID,
		ActionToken:    result.ActionToken,
		NangoConnID:    conn.NangoConnectionID,
		ProviderKey:    providerKey,
	}
}

func (h *NangoWebhookHandler) enqueueGatewaySlackStatus(ctx context.Context, accepted gateway.ConnectionInboundAccepted) {
	if accepted.Envelope.Provider != gateway.SlackProvider || h.enqueuer == nil {
		return
	}
	fields := map[string]any{
		"connection_id": accepted.Envelope.ConnectionID.String(),
		"org_id":        accepted.Envelope.OrgID.String(),
		"employee_id":   accepted.Envelope.EmployeeID.String(),
		"channel_id":    accepted.Inbound.ChannelID,
		"thread_ts":     accepted.Inbound.ThreadID,
		"event_id":      accepted.Event.ID.String(),
	}
	task, err := tasks.NewGatewaySlackStatusTask(tasks.GatewaySlackStatusPayload{
		ConnectionID: accepted.Envelope.ConnectionID.String(),
		OrgID:        accepted.Envelope.OrgID.String(),
		EmployeeID:   accepted.Envelope.EmployeeID.String(),
		ChannelID:    accepted.Inbound.ChannelID,
		ThreadTS:     accepted.Inbound.ThreadID,
		TeamID:       slackInboundTeamID(accepted.Inbound.Raw),
		EventID:      accepted.Event.ID.String(),
		NangoConnID:  accepted.Envelope.NangoConnID,
		ProviderKey:  accepted.Envelope.ProviderKey,
	})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook: build status task: %w", err), fields)
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("slack webhook: enqueue status task: %w", err), fields)
	}
}

func slackInboundTeamID(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	teamID, _ := raw["team_id"].(string)
	return teamID
}

func (h *NangoWebhookHandler) acknowledge(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
