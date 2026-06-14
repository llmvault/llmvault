package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
)

type updateSystemCredentialRequest struct {
	Label          *string    `json:"label,omitempty"`
	ProviderID     *string    `json:"provider_id,omitempty"`
	BaseURL        *string    `json:"base_url,omitempty"`
	AuthScheme     *string    `json:"auth_scheme,omitempty"`
	APIKey         *string    `json:"api_key,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
}

func (h *CredentialHandler) ListSystem(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	q := h.db.Where("org_id IS NULL")
	if provider := r.URL.Query().Get("provider_id"); provider != "" {
		q = q.Where("provider_id = ?", provider)
	}
	q = applyPagination(q, cursor, limit)

	var creds []model.Credential
	if err := q.Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list system credentials"})
		return
	}
	hasMore := len(creds) > limit
	if hasMore {
		creds = creds[:limit]
	}
	resp := make([]credentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = toCredentialResponse(c)
	}
	result := paginatedResponse[credentialResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := creds[len(creds)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CredentialHandler) CreateSystem(w http.ResponseWriter, r *http.Request) {
	var req createCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	cred, err := h.buildCredential(r.Context(), nil, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.db.Create(&cred).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to store system credential", "error", err, "provider_id", req.ProviderID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store system credential"})
		return
	}
	if cred.Remaining != nil && h.counter != nil {
		_ = h.counter.SeedCredential(r.Context(), cred.ID.String(), *cred.Remaining)
	}
	writeJSON(w, http.StatusCreated, toCredentialResponse(cred))
}

func (h *CredentialHandler) UpdateSystem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id required"})
		return
	}
	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id IS NULL", id).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "system credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load system credential"})
		return
	}

	var req updateSystemCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	updates := map[string]any{}
	if req.Label != nil {
		updates["label"] = *req.Label
	}
	if req.ProviderID != nil {
		providerID := *req.ProviderID
		if _, ok := registry.Global().GetProvider(providerID); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider_id %q", providerID)})
			return
		}
		updates["provider_id"] = providerID
	}
	if req.BaseURL != nil {
		if err := proxy.ValidateBaseURL(*req.BaseURL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid base_url: %v", err)})
			return
		}
		updates["base_url"] = *req.BaseURL
	}
	if req.AuthScheme != nil {
		if !validCredentialAuthScheme(*req.AuthScheme) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth_scheme"})
			return
		}
		updates["auth_scheme"] = *req.AuthScheme
	}
	if req.RefillInterval != nil {
		if _, err := time.ParseDuration(*req.RefillInterval); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid refill_interval: must be a valid Go duration (e.g. 1h, 24h)"})
			return
		}
		updates["refill_interval"] = *req.RefillInterval
	}
	if req.Remaining != nil {
		updates["remaining"] = *req.Remaining
	}
	if req.RefillAmount != nil {
		updates["refill_amount"] = *req.RefillAmount
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}
	if req.APIKey != nil {
		encryptedKey, wrappedDEK, err := h.encryptAPIKey(r.Context(), *req.APIKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		updates["encrypted_key"] = encryptedKey
		updates["wrapped_dek"] = wrappedDEK
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, toCredentialResponse(cred))
		return
	}
	if err := h.db.Model(&cred).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update system credential"})
		return
	}
	if h.cacheManager != nil {
		_ = h.cacheManager.InvalidateCredential(r.Context(), cred.ID.String())
	}
	if req.Remaining != nil && h.counter != nil {
		_ = h.counter.SeedCredential(r.Context(), cred.ID.String(), *req.Remaining)
	}
	_ = h.db.First(&cred, "id = ?", cred.ID).Error
	writeJSON(w, http.StatusOK, toCredentialResponse(cred))
}

func (h *CredentialHandler) RevokeSystem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id required"})
		return
	}
	now := time.Now()
	result := h.db.Model(&model.Credential{}).
		Where("id = ? AND org_id IS NULL AND revoked_at IS NULL", id).
		Update("revoked_at", &now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke system credential"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "system credential not found or already revoked"})
		return
	}
	if h.cacheManager != nil {
		_ = h.cacheManager.InvalidateCredential(r.Context(), id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
