package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// List handles GET /v1/api-keys.
// @Summary List API keys
// @Description Returns API keys for the current organization with cursor pagination.
// @Tags api-keys
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[apiKeyResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/api-keys [get]
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ?", org.ID)
	q = applyPagination(q, cursor, limit)

	var keys []model.APIKey
	if err := q.Find(&keys).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list api keys"})
		return
	}

	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}

	items := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		items[i] = apiKeyResponse{
			ID:        k.ID.String(),
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			Scopes:    k.Scopes,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		}
		if k.ExpiresAt != nil {
			s := k.ExpiresAt.Format(time.RFC3339)
			items[i].ExpiresAt = &s
		}
		if k.LastUsedAt != nil {
			s := k.LastUsedAt.Format(time.RFC3339)
			items[i].LastUsedAt = &s
		}
		if k.RevokedAt != nil {
			s := k.RevokedAt.Format(time.RFC3339)
			items[i].RevokedAt = &s
		}
	}

	resp := paginatedResponse[apiKeyResponse]{
		Data:    items,
		HasMore: hasMore,
	}
	if hasMore {
		last := keys[len(keys)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/api-keys/{id}.
// @Summary Revoke an API key
// @Description Soft-deletes an API key by setting its revoked_at timestamp.
// @Tags api-keys
// @Produce json
// @Param id path string true "API Key ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/api-keys/{id} [delete]
func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "api key id required"})
		return
	}

	var apiKey model.APIKey
	if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", keyID, org.ID).First(&apiKey).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found or already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find api key"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&apiKey).Update("revoked_at", &now).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to revoke api key", "error", err, "org_id", org.ID, "key_id", keyID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke api key"})
		return
	}

	h.keyCache.Invalidate(apiKey.KeyHash)

	if err := h.cacheManager.InvalidateAPIKey(r.Context(), apiKey.KeyHash); err != nil {
		// The revocation is durable in Postgres and the local cache is already
		// evicted; a failed cross-instance publish only delays peer eviction
		// until their cache TTL / reconnect purge. Surface it rather than drop.
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to publish api key invalidation", "error", err, "org_id", org.ID, "key_id", keyID)
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "api key revoked", "org_id", org.ID, "key_id", keyID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
