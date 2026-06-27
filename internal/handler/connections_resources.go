package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type updateConnectionResourcesRequest struct {
	Resources map[string][]agentConnectionResourceSelection `json:"resources"`
}

type updateConnectionResourcesResponse struct {
	ConnectionID string     `json:"connection_id"`
	Resources    model.JSON `json:"resources"`
	CloneQueued  bool       `json:"clone_queued"`
}

// @Summary List available resources for a connection
// @Description Fetches available resources of a specific type from the provider API. For example, list all repositories for a GitHub connection.
// @Tags connections
// @Produce json
// @Param id path string true "In-Connection ID"
// @Param type path string true "Resource type (e.g., repository, project)"
// @Success 200 {object} github_com_usehivy_hivy_internal_resources.DiscoveryResult
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/{id}/resources/{type} [get]
func (h *ConnectionHandler) ListResources(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	connID := chi.URLParam(r, "id")
	resourceType := chi.URLParam(r, "type")

	if connID == "" || resourceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id and resource type are required"})
		return
	}

	var conn model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get connection"})
		return
	}

	provider := conn.Integration.Provider
	nangoProviderConfigKey := conn.Integration.UniqueKey

	result, err := h.discovery.Discover(r.Context(), provider, resourceType, nangoProviderConfigKey, conn.NangoConnectionID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "resource discovery failed",
			"connection_id", connID,
			"provider", provider,
			"resource_type", resourceType,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch resources"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// UpdateResources handles PUT /v1/connections/{id}/resources.
// @Summary Save default resources for a connection
// @Description Stores default provider resources such as selected GitHub repositories on the connection. Agent-specific resources can still override these defaults.
// @Tags connections
// @Accept json
// @Produce json
// @Param id path string true "Connection ID"
// @Param body body updateConnectionResourcesRequest true "Selected resources grouped by resource type"
// @Success 200 {object} updateConnectionResourcesResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/connections/{id}/resources [put]
func (h *ConnectionHandler) UpdateResources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "connection id is required"})
		return
	}

	var req updateConnectionResourcesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	var conn model.Connection
	var normalized model.JSON
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("Integration").
			Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
			First(&conn).Error; err != nil {
			return err
		}
		resources, err := normalizeConnectionResources(conn.Integration.Provider, req.Resources)
		if err != nil {
			return err
		}
		meta := model.JSON{}
		for key, value := range conn.Meta {
			meta[key] = value
		}
		if len(resources) == 0 {
			delete(meta, "resources")
		} else {
			meta["resources"] = resources
		}
		if err := tx.Model(&model.Connection{}).
			Where("id = ? AND org_id = ?", conn.ID, org.ID).
			Update("meta", meta).Error; err != nil {
			return fmt.Errorf("save connection resources: %w", err)
		}
		conn.Meta = meta
		normalized = resources
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
		case strings.Contains(err.Error(), "not configurable"), strings.Contains(err.Error(), "missing"):
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		default:
			logging.CaptureWithFields(ctx, fmt.Errorf("update connection resources: %w", err), map[string]any{
				"org_id":        org.ID.String(),
				"connection_id": connID,
			})
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save resources"})
		}
		return
	}

	writeJSON(w, http.StatusOK, updateConnectionResourcesResponse{
		ConnectionID: conn.ID.String(),
		Resources:    normalized,
		CloneQueued:  h.enqueueConnectionDefaultResourceReconcile(ctx, org.ID, conn),
	})
}

func (h *ConnectionHandler) enqueueConnectionDefaultResourceReconcile(ctx context.Context, orgID uuid.UUID, conn model.Connection) bool {
	return false
}
