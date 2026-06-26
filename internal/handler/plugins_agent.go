package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

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
	var orgInstalls []model.OrgPluginInstall
	if err := h.db.Preload("Plugin").
		Where("org_id = ? AND revoked_at IS NULL", org.ID).
		Find(&orgInstalls).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list org plugins"})
		return
	}
	resp := make([]pluginResponse, 0, len(orgInstalls))
	for _, install := range orgInstalls {
		item, err := h.toPluginResponse(r.Context(), org.ID, install.Plugin)
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
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return enablePluginForAgent(ctx, tx, org.ID, agent.ID, plugin.ID)
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enable plugin for agent"})
		return
	}
	resp, err := h.toPluginResponse(ctx, org.ID, plugin)
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
	if pluginstore.PluginAutoInstall(plugin) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plugin is installed for all agents and cannot be disabled"})
		return
	}
	if pluginstore.PluginLocked(plugin) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plugin is required and cannot be disabled"})
		return
	}
	if agent.AgentCatalog != nil && containsString(agent.AgentCatalog.RequiredPlugins, plugin.Slug) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plugin is required for this agent and cannot be disabled"})
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
