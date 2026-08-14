package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
)

// DesktopRuntimeConfig compiles the same runtime payload used by hosted
// sandboxes, but binds its outbound stream to a user-owned desktop runtime
// identity. The desktop client keeps this payload memory-only.
// @Summary Bootstrap an agent in the desktop runtime
// @Tags desktop
// @Produce json
// @Param agentID path string true "Agent ID"
// @Success 200 {object} desktopRuntimeConfigResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/desktop/agents/{agentID}/runtime-config [post]
func (h *SessionHandler) DesktopRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	org, ok := sessionOrg(w, r)
	if !ok {
		return
	}
	userID, ok := currentSessionUserID(r)
	if !ok || userID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	agent, ok := h.resolveSessionAgent(w, r, org.ID, chi.URLParam(r, "agentID"))
	if !ok {
		return
	}
	if !h.canUseTeam(r.Context(), org.ID, agent.TeamID, userID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return
	}
	if h.runtimeEncKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "desktop runtime is not configured"})
		return
	}

	sb, secret, err := h.desktopSandbox(r, org.ID, agent.ID, *userID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "prepare desktop runtime identity", "org_id", org.ID, "agent_id", agent.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to prepare desktop runtime"})
		return
	}
	configUpdate, _, err := agentruntime.BuildAgentRuntimeConfigUpdateWithOptions(
		r.Context(),
		h.compileDeps,
		&agent,
		&sb,
		secret,
		agentruntime.RuntimeConfigOptions{
			TeamID: agent.TeamID,
			MCPContext: agentruntime.MCPRuntimeContext{
				ActorUserID: userID,
				Source:      agentruntime.MCPInvocationWeb,
			},
		},
	)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "compile desktop runtime config", "org_id", org.ID, "agent_id", agent.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to compile desktop runtime configuration"})
		return
	}
	normalizeDesktopRuntimeConfig(&configUpdate)
	// The loopback API bearer belongs to the desktop app's OS keychain. The
	// cloud runtime secret remains inside runtime_env solely for cloud ingress.
	configUpdate.RuntimeSecret = ""
	writeJSON(w, http.StatusOK, desktopRuntimeConfigResponse{
		AgentID:   agent.ID.String(),
		SandboxID: sb.ID.String(),
		Config:    configUpdate,
	})
}

func normalizeDesktopRuntimeConfig(update *agentruntime.ConfigUpdateRequest) {
	if update == nil {
		return
	}
	if update.RuntimeEnv == nil {
		update.RuntimeEnv = make(map[string]string)
	}
	update.RuntimeEnv["HIVY_RUNTIME_MODE"] = "desktop"
	for key, value := range update.RuntimeEnv {
		update.RuntimeEnv[key] = desktopReachableURL(value)
	}
	normalizeDesktopAgentDefinition(update.Definition)
}

func normalizeDesktopAgentDefinition(definition *agentruntime.AgentDefinition) {
	if definition == nil {
		return
	}
	normalizeDesktopModel(&definition.Model)
	for index, server := range definition.McpServers {
		definition.McpServers[index] = normalizeDesktopConfigValue(server)
	}
	for index, channel := range definition.OutboundChannels {
		definition.OutboundChannels[index] = normalizeDesktopConfigValue(channel)
	}
	for _, subagent := range definition.SubAgents {
		normalizeDesktopAgentDefinition(subagent)
	}
}

func normalizeDesktopModel(model *agentruntime.ModelConfig) {
	if model == nil {
		return
	}
	model.BaseURL = desktopReachableURL(model.BaseURL)
	normalizeDesktopModel(model.Fallback)
}

func normalizeDesktopConfigValue(value any) any {
	switch typed := value.(type) {
	case string:
		return desktopReachableURL(typed)
	case map[string]any:
		for key, nested := range typed {
			typed[key] = normalizeDesktopConfigValue(nested)
		}
	case map[string]string:
		for key, nested := range typed {
			typed[key] = desktopReachableURL(nested)
		}
	case []any:
		for index, nested := range typed {
			typed[index] = normalizeDesktopConfigValue(nested)
		}
	}
	return value
}

func desktopReachableURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "host.docker.internal") {
		return value
	}
	authorityStart := strings.Index(value, "://")
	if authorityStart < 0 {
		return value
	}
	authorityStart += len("://")
	authorityEnd := len(value)
	if index := strings.IndexAny(value[authorityStart:], "/?#"); index >= 0 {
		authorityEnd = authorityStart + index
	}
	host := "127.0.0.1"
	if port := parsed.Port(); port != "" {
		host += ":" + port
	}
	return value[:authorityStart] + host + value[authorityEnd:]
}
