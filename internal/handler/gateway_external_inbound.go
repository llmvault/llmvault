package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/gateway"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

// HandleInbound handles POST /incoming/gateways/external/{routeID}.
// @Summary Receive an external gateway message
// @Tags webhooks
// @Accept json
// @Produce json
// @Param routeID path string true "Gateway route ID"
// @Success 202 {object} map[string]any
// @Router /incoming/gateways/external/{routeID} [post]
func (h *GatewayExternalHandler) HandleInbound(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil || h.enqueuer == nil || h.encKey == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "external gateway is not configured"})
		return
	}
	routeID, err := uuid.Parse(chi.URLParam(r, "routeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route_id"})
		return
	}
	route, err := h.loadExternalRoute(r, routeID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "gateway route not found"})
		return
	}
	if !route.Enabled || route.RevokedAt != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "gateway route is not active"})
		return
	}
	if !gateway.VerifyExternalGatewaySecret(bearerFromHeader(r.Header.Get("Authorization")), externalRouteSecretHash(route)) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid gateway secret"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	result, err := h.service.ReceiveWebhook(r.Context(), gateway.WebhookEnvelope{
		RouteID: route.ID,
		Headers: normalizedHeaders(r.Header),
		Body:    body,
	})
	if err != nil {
		logging.CaptureWithFields(r.Context(), fmt.Errorf("external gateway inbound: %w", err), map[string]any{
			"route_id": route.ID.String(), "org_id": route.OrgID.String(), "agent_id": route.AgentID.String(),
		})
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if result == nil || result.Ignored {
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "ignored"})
		return
	}
	if result.Duplicate {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "duplicate",
			"duplicate": true,
			"event_id":  result.Event.ID.String(),
		})
		return
	}
	if err := h.enqueueExternalCallback(r, route, result); err != nil {
		logging.CaptureWithFields(r.Context(), fmt.Errorf("external gateway callback enqueue: %w", err), map[string]any{
			"route_id": route.ID.String(), "event_id": result.Event.ID.String(),
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue response callback"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":             "accepted",
		"duplicate":          false,
		"event_id":           result.Event.ID.String(),
		"agent_session_id":   result.Session.ID.String(),
		"runtime_session_id": result.Runtime.SessionID,
		"runtime_trace_id":   result.Runtime.TraceID,
		"runtime_turn_id":    result.Runtime.TurnID,
	})
}

func (h *GatewayExternalHandler) enqueueExternalCallback(r *http.Request, route model.AgentGatewayRoute, result *gateway.ReceiveResult) error {
	var sandbox model.Sandbox
	if err := h.db.WithContext(r.Context()).Where("id = ?", result.Session.SandboxID).First(&sandbox).Error; err != nil {
		return fmt.Errorf("load gateway session sandbox: %w", err)
	}
	runtimeSecret, err := h.encKey.DecryptString(sandbox.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	routeSecret, err := decryptExternalRouteSecret(h.encKey, route)
	if err != nil {
		return err
	}
	streamURL := absoluteRuntimeURL(sandbox.RuntimeURL, result.Runtime.ResponseStreamURL)
	if streamURL == "" {
		streamURL = strings.TrimRight(sandbox.RuntimeURL, "/") + "/gateway/http/response-streams/" + result.Runtime.ResponseStreamID
	}
	if streamURL == "" && result.Runtime.StreamID != "" {
		streamURL = strings.TrimRight(sandbox.RuntimeURL, "/") + "/gateway/http/streams/" + result.Runtime.StreamID
	}
	task, taskOpts, err := tasks.NewGatewayExternalCallbackTask(tasks.GatewayExternalCallbackPayload{
		RouteID:        route.ID.String(),
		OrgID:          route.OrgID.String(),
		AgentID:        route.AgentID.String(),
		EventID:        result.Event.ID.String(),
		SessionID:      result.Session.ID.String(),
		RuntimeConvoID: result.Session.RuntimeConversationID,
		StreamURL:      streamURL,
		RuntimeURL:     sandbox.RuntimeURL,
		RuntimeAPIKey:  runtimeSecret,
		TraceID:        result.Runtime.TraceID,
		TurnID:         result.Runtime.TurnID,
		Provider:       route.Provider,
		CallbackURL:    externalRouteCallbackURL(route),
		RouteSecret:    routeSecret,
		ThreadKey:      result.Event.ThreadKey,
		ChannelID:      result.Event.ChannelID,
		ThreadID:       result.Event.ThreadID,
	})
	if err != nil {
		return err
	}
	_, err = h.enqueuer.Enqueue(task, taskOpts...)
	return err
}
