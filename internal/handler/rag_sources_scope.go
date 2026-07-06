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
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/rag/connectors/scope"
)

type ragScopesResponse struct {
	Scopes []catalog.ConfigurableResourceSummary `json:"scopes"`
}

// ListConnectionScopes handles GET /v1/rag/connections/{connection_id}/scopes.
// It returns the resource types a RAG source on this connection can be scoped
// to (e.g. Notion pages/databases, GitHub repos, Linear teams). The frontend
// then lists the actual items per type via GET /v1/connections/{id}/resources/{type}.
//
// @Summary List the scope types for a connection
// @Description Returns the resource types a knowledge source on this connection can be scoped to.
// @Tags rag
// @Produce json
// @Param connection_id path string true "Connection ID"
// @Success 200 {object} ragScopesResponse
// @Security BearerAuth
// @Router /v1/rag/connections/{connection_id}/scopes [get]
func (h *RAGSourceHandler) ListConnectionScopes(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	connID, err := uuid.Parse(chi.URLParam(r, "connection_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid connection id"})
		return
	}

	var conn model.Connection
	if err := h.db.Preload("Integration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load connection"})
		return
	}

	scopes := []catalog.ConfigurableResourceSummary{}
	if h.catalog != nil {
		scopes = h.catalog.GetRAGScopableResources(conn.Integration.Provider)
		if scopes == nil {
			scopes = []catalog.ConfigurableResourceSummary{}
		}
	}
	writeJSON(w, http.StatusOK, ragScopesResponse{Scopes: scopes})
}

// validateScope checks the config.scope selection on an INTEGRATION source.
// A source with no scope indexes everything the connection can access and needs
// no validation. When a scope is present it must name a resource type the
// provider marks rag_scopable, and every selected id must actually belong to
// the connection (a membership check against the provider via Nango).
//
// The membership check is best-effort: if discovery is unavailable (e.g. the
// provider API is momentarily down) we log and allow, so a transient outage
// can't block source creation. Returns (0, "") when valid, else an HTTP status
// and message.
func (h *RAGSourceHandler) validateScope(ctx context.Context, conn *model.Connection, config json.RawMessage) (int, string) {
	if conn == nil {
		return 0, ""
	}
	sc, present, err := scope.Parse(config)
	if err != nil {
		return http.StatusUnprocessableEntity, err.Error()
	}
	if !present {
		return 0, ""
	}

	provider := conn.Integration.Provider
	if sc.ResourceType == "" {
		return http.StatusUnprocessableEntity, "scope.resource_type is required when a scope is provided"
	}
	if h.catalog == nil || !h.catalog.IsRAGScopable(provider, sc.ResourceType) {
		return http.StatusUnprocessableEntity, fmt.Sprintf(
			"resource type %q is not a valid knowledge scope for %s", sc.ResourceType, provider)
	}

	ids := sc.IDs()
	if len(ids) == 0 {
		return http.StatusUnprocessableEntity, "scope must select at least one resource"
	}

	if h.discovery == nil {
		return 0, ""
	}
	result, err := h.discovery.Discover(ctx, provider, sc.ResourceType, conn.Integration.UniqueKey, conn.NangoConnectionID)
	if err != nil {
		// Best-effort: don't fail creation on a transient discovery outage.
		logging.FromContext(ctx).WarnContext(ctx, "rag scope membership check skipped: discovery failed",
			"provider", provider, "resource_type", sc.ResourceType, "connection_id", conn.ID, "error", err)
		return 0, ""
	}
	allowed := make(map[string]struct{}, len(result.Resources))
	for _, res := range result.Resources {
		allowed[res.ID] = struct{}{}
	}
	var missing []string
	for _, id := range ids {
		if _, ok := allowed[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return http.StatusUnprocessableEntity, fmt.Sprintf(
			"these %s are not accessible via this connection: %s", sc.ResourceType, strings.Join(missing, ", "))
	}
	return 0, ""
}
