package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

var employeeAllowedModelIDs = []string{
	"deepseek-v4-flash",
	"step-3.7-flash",
	"ling-2.6-1t",
	"mimo-v2.5-pro",
}

// ListModels handles GET /v1/employees/models.
// @Summary List employee-selectable models
// @Description Returns the OpenRouter-backed model allowlist supported for Hivy employees.
// @Tags employees
// @Produce json
// @Success 200 {array} modelSummary
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/models [get]
func (h *EmployeeHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.employeeModelSummaries(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load employee models"})
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func (h *EmployeeHandler) employeeModelRegistry() *registry.Registry {
	if h != nil && h.registry != nil {
		return h.registry
	}
	return registry.Global()
}

func (h *EmployeeHandler) employeeModelSummaries(ctx context.Context) ([]modelSummary, error) {
	if h == nil || h.db == nil {
		return []modelSummary{}, nil
	}
	hasOpenRouter, err := hasActiveSystemCredentialForProvider(ctx, h.db, "openrouter")
	if err != nil {
		return nil, err
	}
	if !hasOpenRouter {
		return []modelSummary{}, nil
	}

	reg := h.employeeModelRegistry()
	out := make([]modelSummary, 0, len(employeeAllowedModelIDs))
	for _, id := range employeeAllowedModelIDs {
		route, ok := reg.ResolveModel("openrouter", id)
		if !ok {
			continue
		}
		mdl := route.Model
		out = append(out, modelSummary{
			ID:               id,
			Name:             mdl.Name,
			ProviderIDs:      []string{"openrouter"},
			Family:           mdl.Family,
			Reasoning:        mdl.Reasoning,
			ToolCall:         mdl.ToolCall,
			StructuredOutput: mdl.StructuredOutput,
			OpenWeights:      mdl.OpenWeights,
			Knowledge:        mdl.Knowledge,
			ReleaseDate:      mdl.ReleaseDate,
			Modalities:       mdl.Modalities,
			Cost:             mdl.Cost,
			Limit:            mdl.Limit,
			Status:           mdl.Status,
			Speed:            mdl.Speed,
			Description:      mdl.Description,
		})
	}
	return out, nil
}

func hasActiveSystemCredentialForProvider(ctx context.Context, db *gorm.DB, providerID string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).
		Model(&model.Credential{}).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", providerID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count active system credentials: %w", err)
	}
	return count > 0, nil
}

func pickActiveSystemCredentialForModel(ctx context.Context, db *gorm.DB, reg *registry.Registry, modelID string) (*model.Credential, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("model is required")
	}
	if reg == nil {
		reg = registry.Global()
	}

	if err := reg.ValidateCanonicalModel(modelID); err != nil {
		return nil, err
	}

	var creds []model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL").
		Order("created_at ASC").
		Find(&creds).Error; err != nil {
		return nil, fmt.Errorf("list active system credentials: %w", err)
	}

	for i := range creds {
		if _, ok := reg.ResolveModel(creds[i].ProviderID, modelID); ok {
			return &creds[i], nil
		}
	}
	return nil, fmt.Errorf("model %q is not backed by an active system credential", modelID)
}
