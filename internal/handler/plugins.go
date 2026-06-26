package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
	"github.com/usehivy/hivy/internal/tasks"
)

type PluginHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
	catalog  *catalog.Catalog
}

func NewPluginHandler(db *gorm.DB, enqueuers ...enqueue.TaskEnqueuer) *PluginHandler {
	var enq enqueue.TaskEnqueuer
	if len(enqueuers) > 0 {
		enq = enqueuers[0]
	}
	return &PluginHandler{db: db, enqueuer: enq, catalog: catalog.Global()}
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
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var plugins []model.Plugin
	if err := h.db.Where("status = ?", model.PluginStatusActive).Order("name ASC").Find(&plugins).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list plugins"})
		return
	}
	resp := make([]pluginResponse, 0, len(plugins))
	for _, plugin := range plugins {
		item, err := h.toPluginResponse(r.Context(), org.ID, plugin)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin details"})
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
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	resp, err := h.toPluginResponse(r.Context(), org.ID, plugin)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin details"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary Install plugin
// @Description Installs a plugin for the current organization and enables it for eligible non-default agents when requirements are satisfied.
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
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	user, _ := middleware.UserFromContext(ctx)
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	missing, err := h.missingRequirements(ctx, org.ID, plugin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check plugin requirements"})
		return
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":                "plugin requirements are not connected",
			"missing_requirements": missing,
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
		return pluginstore.EnsureInstalledPluginForEligibleAgents(ctx, tx, org.ID, plugin)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to install plugin"})
		return
	}
	if err := tasks.EnqueuePluginInstallSync(ctx, h.enqueuer, tasks.PluginInstallSyncPayload{
		OrgID:     org.ID,
		PluginID:  plugin.ID,
		InstallID: install.ID,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to queue plugin install sync"})
		return
	}
	resp, err := h.toPluginResponse(ctx, org.ID, plugin)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin details"})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// @Summary Uninstall plugin
// @Description Uninstalls a plugin for the current organization and removes it from enabled agents.
// @Tags plugins
// @Produce json
// @Param slug path string true "Plugin slug"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/plugins/{slug}/install [delete]
func (h *PluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	if pluginstore.PluginAutoInstall(plugin) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plugin is installed for all agents and cannot be uninstalled"})
		return
	}
	if pluginstore.PluginLocked(plugin) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plugin is required and cannot be uninstalled"})
		return
	}
	now := time.Now()
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OrgPluginInstall{}).
			Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}
		return disablePluginForOrg(ctx, tx, org.ID, plugin.ID)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to uninstall plugin"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}
