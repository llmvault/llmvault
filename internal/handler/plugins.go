package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

type PluginHandler struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

func NewPluginHandler(db *gorm.DB) *PluginHandler {
	return &PluginHandler{db: db, catalog: catalog.Global()}
}

// @Summary List plugins
// @Description Returns active plugins for the current organization, including install state, requirements, and presentation metadata for the plugins catalog.
// @Tags plugins
// @Produce json
// @Success 200 {array} pluginResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/plugins [get]
func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	scope, err := h.actorScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	var plugins []model.Plugin
	if err := h.db.WithContext(r.Context()).Where("status = ? AND (org_id IS NULL OR org_id = ?)", model.PluginStatusActive, org.ID).Order("name ASC").Find(&plugins).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "list plugins", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list plugins"})
		return
	}
	// Batch every per-plugin lookup with `plugin_id IN (?)` and memoize the
	// org-scoped connection checks so a list of N plugins costs a bounded number
	// of queries instead of the previous 1+~6N.
	batch, err := h.loadPluginListData(r.Context(), org.ID, plugins, scope)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load plugin list data", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load plugin details"})
		return
	}
	cache := newPluginConnCache(h, org.ID)
	resp := make([]pluginResponse, 0, len(plugins))
	for _, plugin := range plugins {
		item, err := h.assemblePluginResponse(r.Context(), org.ID, plugin, batch.skills[plugin.ID], batch.reqs[plugin.ID], batch.installCount[plugin.ID], batch.enabled[plugin.ID], cache)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "assemble plugin response", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load plugin details"})
			return
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Get plugin
// @Description Returns one active plugin by slug for the current organization, including install state, requirements, and presentation metadata.
// @Tags plugins
// @Produce json
// @Param slug path string true "Plugin slug"
// @Success 200 {object} pluginResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/plugins/{slug} [get]
func (h *PluginHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	scope, err := h.actorScope(r.Context(), org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	resp, err := h.toPluginResponse(r.Context(), org.ID, plugin, scope)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load plugin details", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load plugin details"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Install plugin
// @Description Installs a plugin for the current organization when requirements are satisfied. Agents receive it through team grants.
// @Tags plugins
// @Produce json
// @Param slug path string true "Plugin slug"
// @Success 201 {object} pluginResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} pluginInstallConflictResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/plugins/{slug}/install [post]
func (h *PluginHandler) Install(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	user, _ := middleware.UserFromContext(ctx)
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	missing, err := h.missingRequirements(ctx, org.ID, plugin.ID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "check plugin requirements", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check plugin requirements"})
		return
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusConflict, pluginInstallConflictResponse{
			Error:               "plugin requirements are not connected",
			MissingRequirements: missing,
		})
		return
	}
	var install model.OrgPluginInstall
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.OrgPluginInstall
		err := tx.Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).First(&existing).Error
		if err == nil {
			install = existing
		} else if err == gorm.ErrRecordNotFound {
			var createdBy *uuid.UUID
			if user != nil && user.ID != uuid.Nil {
				createdBy = &user.ID
			}
			install = model.OrgPluginInstall{
				ID:              uuid.New(),
				OrgID:           org.ID,
				PluginID:        plugin.ID,
				CreatedByUserID: createdBy,
			}
			if err := tx.Create(&install).Error; err != nil {
				return fmt.Errorf("create org plugin install: %w", err)
			}
		} else {
			return fmt.Errorf("load org plugin install: %w", err)
		}
		return pluginstore.RefreshPluginSkillInstallCounts(ctx, tx, plugin.ID)
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "install plugin", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to install plugin"})
		return
	}
	scope, err := h.actorScope(ctx, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	resp, err := h.toPluginResponse(ctx, org.ID, plugin, scope)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load plugin details", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load plugin details"})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// @Summary Uninstall plugin
// @Description Uninstalls a plugin for the current organization and removes its team grants.
// @Tags plugins
// @Produce json
// @Param slug path string true "Plugin slug"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/plugins/{slug}/install [delete]
func (h *PluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	if pluginstore.PluginAutoInstall(plugin) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "plugin is installed for all agents and cannot be uninstalled"})
		return
	}
	if pluginstore.PluginLocked(plugin) {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "plugin is required and cannot be uninstalled"})
		return
	}
	required, err := pluginRequiredByOrgAgents(ctx, h.db, org.ID, plugin.Slug)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "check plugin requirement", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check plugin requirement"})
		return
	}
	if required {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "plugin is required by an active agent and cannot be uninstalled"})
		return
	}
	now := time.Now()
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OrgPluginInstall{}).
			Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}
		return disablePluginForOrg(ctx, tx, org.ID, plugin.ID)
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "uninstall plugin", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to uninstall plugin"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "uninstalled"})
}
