package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/proxy"
	"github.com/usehivy/hivy/internal/registry"
	llm "github.com/usehivy/hivy/internal/trigger/hivy"
)

const credentialTestTimeout = 30 * time.Second

type testSystemCredentialRequest struct {
	ProviderID string `json:"provider_id"`
	BaseURL    string `json:"base_url"`
	AuthScheme string `json:"auth_scheme"`
	APIKey     string `json:"api_key"`
}

type testSystemCredentialResponse struct {
	Status     string `json:"status"`
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

// TestSystem handles POST /v1/admin/system-credentials/test.
// @Summary Test a system credential
// @Description Runs a minimal model inference with an unsaved credential.
// @Tags admin
// @Accept json
// @Produce json
// @Param X-Hivy-Admin-Secret header string true "Admin secret"
// @Param body body testSystemCredentialRequest true "Unsaved credential details"
// @Success 200 {object} testSystemCredentialResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/admin/system-credentials/test [post]
func (h *CredentialHandler) TestSystem(w http.ResponseWriter, r *http.Request) {
	var req testSystemCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	credential, apiKey, modelID, ok := normalizeSystemCredentialTest(w, req)
	if !ok {
		return
	}

	factory := h.testClient
	if factory == nil {
		factory = llm.NewCompletionClient
	}
	ctx, cancel := context.WithTimeout(r.Context(), credentialTestTimeout)
	defer cancel()
	_, err := factory(&credential, apiKey).ChatCompletion(ctx, llm.CompletionRequest{
		Model: modelID,
		Messages: []llm.Message{
			{Role: "user", Content: "Reply with only OK."},
		},
		MaxTokens: 8,
	})
	if err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "credential connection test failed",
			"error", err,
			"provider_id", credential.ProviderID,
			"model_id", modelID,
		)
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "connection test failed; verify the API key, base URL, and provider account"})
		return
	}

	writeJSON(w, http.StatusOK, testSystemCredentialResponse{
		Status:     "ok",
		ProviderID: credential.ProviderID,
		ModelID:    modelID,
	})
}

func normalizeSystemCredentialTest(w http.ResponseWriter, req testSystemCredentialRequest) (model.Credential, string, string, bool) {
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.AuthScheme = strings.TrimSpace(req.AuthScheme)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.ProviderID == "" || req.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "provider_id and api_key are required"})
		return model.Credential{}, "", "", false
	}

	provider, exists := registry.Global().GetProvider(req.ProviderID)
	if !exists {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "unknown provider_id"})
		return model.Credential{}, "", "", false
	}
	if req.BaseURL == "" {
		req.BaseURL = provider.API
	}
	if req.BaseURL == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "base_url is required for this provider"})
		return model.Credential{}, "", "", false
	}
	if err := proxy.ValidateBaseURL(req.BaseURL); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid base_url"})
		return model.Credential{}, "", "", false
	}
	if req.AuthScheme == "" {
		req.AuthScheme = defaultCredentialAuthScheme(req.ProviderID)
	}
	if !validCredentialAuthScheme(req.AuthScheme) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid auth_scheme"})
		return model.Credential{}, "", "", false
	}

	modelID := credentialTestModelID(*provider)
	if modelID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "provider does not expose a testable text inference model"})
		return model.Credential{}, "", "", false
	}

	return model.Credential{
		ProviderID: req.ProviderID,
		BaseURL:    req.BaseURL,
		AuthScheme: req.AuthScheme,
	}, req.APIKey, modelID, true
}
