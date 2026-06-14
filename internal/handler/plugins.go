package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type PluginHandler struct {
	db *gorm.DB
}

func NewPluginHandler(db *gorm.DB) *PluginHandler {
	return &PluginHandler{db: db}
}

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

func (h *PluginHandler) Install(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	user, _ := middleware.UserFromContext(r.Context())
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	missing, err := h.missingRequirements(r.Context(), org.ID, plugin.ID)
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
	err = h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
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
		agent, err := ensureHivyAgent(r.Context(), tx, org.ID)
		if err != nil {
			return err
		}
		return enablePluginForAgent(r.Context(), tx, org.ID, agent.ID, plugin.ID)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to install plugin"})
		return
	}
	resp, err := h.toPluginResponse(r.Context(), org.ID, plugin)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load plugin details"})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *PluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	plugin, ok := h.loadPluginBySlug(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	now := time.Now()
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OrgPluginInstall{}).
			Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}
		return disablePluginForOrg(r.Context(), tx, org.ID, plugin.ID)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to uninstall plugin"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled"})
}
