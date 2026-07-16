package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// WithAppsService enables the app preview-env side channel (nil leaves the
// endpoint answering 503, matching the other optional upload features).
func (h *UploadsHandler) WithAppsService(svc *apps.Service) *UploadsHandler {
	h.appsService = svc
	return h
}

// AppPreviewEnv hands a builder agent everything it needs to run a preview of
// an app inside its OWN sandbox: the app's full deploy env (plus PORT) and the
// server-computed public URL for the requested port. Auth is the sandbox
// runtime-secret bearer (authAgent); the app must be an active app of the
// agent's org.
//
// SECURITY: the env values are secrets. They exist only in this HTTP response
// (fetched by `make preview` in the sandbox, never surfaced to the model) —
// nothing here may log or echo them.
//
//	GET /internal/agents/{agentID}/sandboxes/{sandboxID}/apps/{appID}/preview-env?port=NNNN
func (h *UploadsHandler) AppPreviewEnv(w http.ResponseWriter, r *http.Request) {
	if h.appsService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "apps preview is not configured"})
		return
	}
	agent, sb, ok := h.authAgent(w, r)
	if !ok {
		return
	}
	appID, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid app_id"})
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("port")))
	if err != nil || port < 1 || port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "port query parameter must be a port number (1-65535)"})
		return
	}

	app, err := h.appsService.GetApp(r.Context(), *agent.OrgID, appID)
	if errors.Is(err, apps.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load app"})
		return
	}
	if app.CreatedByAgentID == nil || *app.CreatedByAgentID != agent.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
		return
	}
	previewURL, status, errMsg := h.appPreviewURL(r.Context(), sb, port)
	if errMsg != "" {
		writeJSON(w, status, map[string]string{"error": errMsg})
		return
	}
	env, err := h.appsService.BuildPreviewEnv(r.Context(), app, port)
	if err != nil {
		// Log the error only — it names variables, never values.
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "build app preview env",
			"agent_id", agent.ID, "app_id", app.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build preview environment"})
		return
	}

	now := time.Now().UTC()
	if err := h.db.WithContext(r.Context()).Model(&model.App{}).
		Where("id = ? AND org_id = ?", app.ID, *agent.OrgID).
		Updates(map[string]any{"preview_url": previewURL, "preview_registered_at": now}).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "register app preview url",
			"agent_id", agent.ID, "app_id", app.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to register preview"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"preview_url": previewURL,
		"env":         env,
	})
}

// appPreviewURL computes the externally reachable URL for a port on the
// builder's sandbox. Microsandbox previews follow the wildcard gateway
// pattern https://{port}-{externalID}.{previewBaseDomain} (the same shape the
// control plane's previewURLs and the system prompt derive); every other
// provider resolves through GetEndpoint with the docker control-origin
// rewrite. Returns (url, 0, "") on success or ("", status, message).
func (h *UploadsHandler) appPreviewURL(ctx context.Context, sb *model.Sandbox, port int) (string, int, string) {
	externalID := strings.TrimSpace(sb.ExternalID)
	if externalID == "" {
		return "", http.StatusConflict, "sandbox has no live runtime to preview from"
	}
	providerID := strings.TrimSpace(sb.ProviderID)
	if strings.EqualFold(providerID, sandbox.ProviderMicrosandbox) {
		baseDomain := previewBaseDomainFromRuntimeURL(sb.RuntimeURL, externalID)
		if baseDomain == "" {
			baseDomain = h.previewBaseDomain
		}
		return fmt.Sprintf("https://%d-%s.%s", port, externalID, baseDomain), 0, ""
	}

	url, err := h.appsService.SandboxEndpointURL(ctx, sb, port)
	if err == nil {
		return url, 0, ""
	}
	if strings.EqualFold(providerID, sandbox.ProviderDocker) {
		published := h.publishedSandboxPorts(ctx, sb)
		if len(published) == 0 {
			return "", http.StatusBadRequest, fmt.Sprintf(
				"port %d is not published on this sandbox (docker provider publishes ports only at creation); no preview ports are published", port)
		}
		return "", http.StatusBadRequest, fmt.Sprintf(
			"port %d is not published on this sandbox (docker provider publishes ports only at creation); published ports: %s",
			port, joinPorts(published))
	}
	logging.FromContext(ctx).ErrorContext(ctx, "resolve sandbox preview endpoint",
		"sandbox_id", sb.ID, "port", port, "error", err)
	return "", http.StatusBadGateway, "failed to resolve the sandbox endpoint for this port"
}

// publishedSandboxPorts probes which of the sandbox's configured preview
// ports actually resolve to a host binding — only called on the docker error
// path, so the extra inspects are fine.
func (h *UploadsHandler) publishedSandboxPorts(ctx context.Context, sb *model.Sandbox) []int {
	candidates, err := model.NormalizeSandboxExposedPorts(model.SandboxExposedPortsFromInt64Array(sb.ExposedPorts))
	if err != nil || len(candidates) == 0 {
		candidates = model.DefaultSandboxExposedPorts()
	}
	var published []int
	for _, candidate := range candidates {
		if _, err := h.appsService.SandboxEndpointURL(ctx, sb, candidate); err == nil {
			published = append(published, candidate)
		}
	}
	return published
}

func joinPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	return strings.Join(parts, ", ")
}
