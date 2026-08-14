package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/registry"
)

type adminLLMProviderResponse struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	BaseURL           string   `json:"base_url,omitempty"`
	DefaultAuthScheme string   `json:"default_auth_scheme"`
	ModelIDs          []string `json:"model_ids"`
	TestModelID       string   `json:"test_model_id"`
}

// ListLLMProviders handles GET /v1/admin/llm-providers.
// @Summary List LLM providers
// @Description Returns available LLM provider definitions for system credential setup.
// @Tags admin
// @Produce json
// @Param X-Hivy-Admin-Secret header string true "Admin secret"
// @Success 200 {array} adminLLMProviderResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/admin/llm-providers [get]
func (h *CredentialHandler) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	providers := registry.Global().AllProviders()
	resp := make([]adminLLMProviderResponse, 0, len(providers))
	for _, provider := range providers {
		modelIDs := make([]string, 0, len(provider.Models))
		for id := range provider.Models {
			modelIDs = append(modelIDs, id)
		}
		sortStrings(modelIDs)
		resp = append(resp, adminLLMProviderResponse{
			ID:                provider.ID,
			Name:              provider.Name,
			BaseURL:           provider.API,
			DefaultAuthScheme: defaultCredentialAuthScheme(provider.ID),
			ModelIDs:          modelIDs,
			TestModelID:       credentialTestModelID(provider),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func credentialTestModelID(provider registry.Provider) string {
	selectedID := ""
	selectedCost := 0.0
	for id, candidate := range provider.Models {
		if !candidate.ToolCall || !modelSupportsTextInput(candidate) || !modelSupportsTextOutput(candidate) {
			continue
		}
		cost := 0.0
		if candidate.Cost != nil {
			cost = candidate.Cost.Input + candidate.Cost.Output
		}
		if selectedID == "" || cost < selectedCost || (cost == selectedCost && id < selectedID) {
			selectedID = id
			selectedCost = cost
		}
	}
	return selectedID
}

func modelSupportsTextInput(candidate registry.Model) bool {
	if candidate.Modalities == nil {
		return false
	}
	return hasModality(candidate.Modalities.Input, "text")
}

func modelSupportsTextOutput(candidate registry.Model) bool {
	if candidate.Modalities == nil {
		return false
	}
	return hasModality(candidate.Modalities.Output, "text")
}

func hasModality(modalities []string, want string) bool {
	for _, modality := range modalities {
		if modality == want {
			return true
		}
	}
	return false
}

func defaultCredentialAuthScheme(providerID string) string {
	if scheme, ok := providerAuthSchemes[providerID]; ok {
		return scheme
	}
	return "bearer"
}

func validCredentialAuthScheme(scheme string) bool {
	switch scheme {
	case "bearer", "x-api-key", "xi-api-key", "api-key", "query_param":
		return true
	default:
		return false
	}
}

func sortStrings(values []string) {
	if len(values) < 2 {
		return
	}
	for i := 0; i < len(values)-1; i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
