package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
	"github.com/usehivy/hivy/internal/teamprovision"
)

// pluginEnabledForTeam reports whether plugin P has been provisioned to team T
// (a team_plugins row). Backed by the single source of truth in
// internal/teamprovision; overridable in tests.
var pluginEnabledForTeam = teamprovision.PluginEnabledForTeam

// authorizeAgentPluginMutation gates enabling/disabling a plugin on an agent on
// the actor being able to manage the agent's team. Agents with team_id IS NULL
// (the org-level Hivy default and legacy unassigned agents) are manager-only:
// there is no team to authorize a plain member against. API-key callers are
// trusted org-wide. Writes a 403/500 and returns false when not allowed.
func (h *PluginHandler) authorizeAgentPluginMutation(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, agent model.Agent) bool {
	// API-key callers and requests with no human actor (system/internal; the
	// route is behind auth middleware that guarantees an authenticated principal
	// upstream) are trusted org-wide, mirroring the agent-write gate.
	if isAPIKeyRequest(ctx) {
		return true
	}
	userID, hasUser := currentRequestUserID(ctx)
	if !hasUser {
		return true
	}
	role, err := orgRoleForUser(ctx, h.db, orgID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve access"})
		return false
	}
	if agent.TeamID == nil {
		if isOrgManager(role) {
			return true
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only an org admin can manage plugins on an agent that is not assigned to a team"})
		return false
	}
	if canManageTeamResource(ctx, h.db, orgID, userID, role, *agent.TeamID) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "you must be a member of the agent's team to manage its plugins"})
	return false
}

// @Summary List agent plugins
// @Description Returns plugins installed in the current organization and their enablement state for one installed agent.
// @Tags plugins
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {array} pluginResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/plugins [get]
func (h *PluginHandler) ListAgentPlugins(w http.ResponseWriter, r *http.Request) {
	org, agent, ok := h.loadAgentFromRoute(w, r)
	if !ok {
		return
	}
	var installs []model.AgentPluginInstall
	if err := h.db.Where("org_id = ? AND agent_id = ?", org.ID, agent.ID).Find(&installs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list agent plugins"})
		return
	}
	enabled := map[uuid.UUID]bool{}
	for _, install := range installs {
		enabled[install.PluginID] = true
	}
	scope, err := h.actorScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve access"})
		return
	}
	var orgInstalls []model.OrgPluginInstall
	if err := h.db.Preload("Plugin").
		Where("org_id = ? AND revoked_at IS NULL", org.ID).
		Find(&orgInstalls).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list org plugins"})
		return
	}
	resp := make([]pluginResponse, 0, len(orgInstalls))
	for _, install := range orgInstalls {
		item, err := h.toPluginResponse(r.Context(), org.ID, install.Plugin, scope)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin details"})
			return
		}
		if !enabled[install.PluginID] {
			item.EnabledAgentIDs = nil
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Enable plugin for agent
// @Description Enables an organization-installed plugin for one installed agent.
// @Tags plugins
// @Produce json
// @Param id path string true "Agent ID"
// @Param slug path string true "Plugin slug"
// @Success 200 {object} pluginResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/plugins/{slug} [post]
func (h *PluginHandler) EnableForAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, agent, ok := h.loadAgentFromRoute(w, r)
	if !ok {
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	// Enabling a plugin on an agent is a team-member action, gated on the actor
	// being able to manage the agent's team. This handler gate is the real
	// authorization — the route is member-reachable.
	if !h.authorizeAgentPluginMutation(ctx, w, org.ID, agent) {
		return
	}
	var count int64
	if err := h.db.Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check org plugin install"})
		return
	}
	if count == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plugin must be installed for the org before enabling it on an agent"})
		return
	}
	// Per-agent plugins are bounded by the agent's team grant: an agent may only
	// receive a plugin its team has been provisioned. Org-level agents (team_id
	// IS NULL, e.g. the Hivy default) skip this — they are manager-only above and
	// have no team allowlist to bound against. Auto-install system plugins
	// (sheets, service-discovery, skill-manager, runtime, …) are always-on for
	// every team without a team_plugins row, so they bypass the grant check —
	// only optional catalog plugins need an explicit team grant.
	if agent.TeamID != nil && !pluginstore.PluginAutoInstall(plugin) {
		granted, err := pluginEnabledForTeam(ctx, h.db, *agent.TeamID, plugin.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check team plugin grant"})
			return
		}
		if !granted {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "plugin is not enabled for the agent's team"})
			return
		}
	}
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return enablePluginForAgent(ctx, tx, org.ID, agent.ID, plugin.ID)
	}); err != nil {
		if errors.Is(err, pluginstore.ErrGitHubIdentityExclusive) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": pluginstore.ErrGitHubIdentityExclusive.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enable plugin for agent"})
		return
	}
	scope, err := h.actorScope(ctx, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve access"})
		return
	}
	resp, err := h.toPluginResponse(ctx, org.ID, plugin, scope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin details"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Disable plugin for agent
// @Description Disables a plugin for one installed agent unless the plugin is required for that agent.
// @Tags plugins
// @Produce json
// @Param id path string true "Agent ID"
// @Param slug path string true "Plugin slug"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{id}/plugins/{slug} [delete]
func (h *PluginHandler) DisableForAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, agent, ok := h.loadAgentFromRoute(w, r)
	if !ok {
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	// Disabling a plugin on an agent is gated on managing the agent's team.
	if !h.authorizeAgentPluginMutation(ctx, w, org.ID, agent) {
		return
	}
	var catalogRequired []string
	if agent.AgentCatalog != nil {
		catalogRequired = []string(agent.AgentCatalog.RequiredPlugins)
	}
	if locked, reason := pluginstore.PluginDetachLock(plugin, agent.IsDefault, catalogRequired); locked {
		writeJSON(w, http.StatusConflict, map[string]string{"error": reason})
		return
	}
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return disablePluginForAgent(ctx, tx, org.ID, agent.ID, plugin.ID)
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to disable plugin for agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}
