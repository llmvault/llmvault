package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type APIKeyHandler struct {
	db           *gorm.DB
	keyCache     *cache.APIKeyCache
	cacheManager *cache.Manager
}

func NewAPIKeyHandler(db *gorm.DB, keyCache *cache.APIKeyCache, cm *cache.Manager) *APIKeyHandler {
	return &APIKeyHandler{db: db, keyCache: keyCache, cacheManager: cm}
}

type createAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresIn *string  `json:"expires_in,omitempty"`
}

type createAPIKeyResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Key       string   `json:"key"`
	KeyPrefix string   `json:"key_prefix"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
	CreatedAt string   `json:"created_at"`
}

type apiKeyResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	KeyPrefix  string   `json:"key_prefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  *string  `json:"expires_at,omitempty"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	RevokedAt  *string  `json:"revoked_at,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

// Create handles POST /v1/api-keys.
// @Summary Create an API key
// @Description Creates a new API key for the current organization. The plaintext key is returned once.
// @Tags api-keys
// @Accept json
// @Produce json
// @Param body body createAPIKeyRequest true "API key parameters"
// @Success 201 {object} createAPIKeyResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/api-keys [post]
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if len(req.Scopes) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one scope is required"})
		return
	}

	for _, s := range req.Scopes {
		if !model.ValidAPIKeyScopes[s] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scope: " + s})
			return
		}
	}

	// Privilege-escalation guard: an API key must never mint a key with broader scopes than its
	// own. JWT callers are already org-admin-gated on the route and may mint any valid scope.
	if apiClaims, ok := middleware.APIKeyClaimsFromContext(r.Context()); ok {
		if !scopesWithinCeiling(req.Scopes, apiClaims.Scopes) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "api key cannot mint a key with broader scopes than its own"})
			return
		}
	}

	plaintext, hash, prefix, err := model.GenerateAPIKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate api key"})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		dur, err := time.ParseDuration(*req.ExpiresIn)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expires_in: must be a valid Go duration (e.g. 720h)"})
			return
		}
		t := time.Now().Add(dur)
		expiresAt = &t
	}

	apiKey := model.APIKey{
		ID:        uuid.New(),
		OrgID:     org.ID,
		Name:      req.Name,
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    req.Scopes,
		ExpiresAt: expiresAt,
		CreatedBy: apiKeyCreatorUserID(r.Context()),
	}

	if err := h.db.Create(&apiKey).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create api key", "error", err, "org_id", org.ID, "name", req.Name)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create api key"})
		return
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "api key created", "org_id", org.ID, "key_id", apiKey.ID, "name", req.Name, "scopes", req.Scopes)

	resp := createAPIKeyResponse{
		ID:        apiKey.ID.String(),
		Name:      apiKey.Name,
		Key:       plaintext,
		KeyPrefix: apiKey.KeyPrefix,
		Scopes:    apiKey.Scopes,
		CreatedAt: apiKey.CreatedAt.Format(time.RFC3339),
	}
	if apiKey.ExpiresAt != nil {
		s := apiKey.ExpiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &s
	}

	writeJSON(w, http.StatusCreated, resp)
}

// apiKeyCreatorUserID resolves the human who minted a key from the request's JWT
// claims. It returns nil for API-key-authenticated callers (machine-to-machine,
// no human actor) and when the claims carry no parseable user id, matching the
// nullable created_by column.
func apiKeyCreatorUserID(ctx context.Context) *uuid.UUID {
	claims, ok := middleware.AuthClaimsFromContext(ctx)
	if !ok || claims == nil || claims.UserID == "" {
		return nil
	}
	id, err := uuid.Parse(claims.UserID)
	if err != nil || id == uuid.Nil {
		return nil
	}
	return &id
}

// scopesWithinCeiling reports whether every requested scope is permitted given
// the caller's own scopes. A caller holding "all" may mint any scope; otherwise
// each requested scope must be one the caller already holds.
func scopesWithinCeiling(requested, callerScopes []string) bool {
	held := make(map[string]bool, len(callerScopes))
	for _, s := range callerScopes {
		held[s] = true
	}
	if held["all"] {
		return true
	}
	for _, s := range requested {
		if !held[s] {
			return false
		}
	}
	return true
}
