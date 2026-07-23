package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/orgtier"
)

type OrgHandler struct {
	db  *gorm.DB
	enq enqueue.TaskEnqueuer
}

func NewOrgHandler(db *gorm.DB, enq enqueue.TaskEnqueuer) *OrgHandler {
	return &OrgHandler{db: db, enq: enq}
}

func (h *OrgHandler) buildOrgResponse(org model.Org) orgResponse {
	sandboxExposedPorts, err := model.NormalizeSandboxExposedPorts(model.SandboxExposedPortsFromInt64Array(org.SandboxExposedPorts))
	if err != nil {
		sandboxExposedPorts = model.DefaultSandboxExposedPorts()
	}
	limits := orgtier.LimitsForTier(org.CapacityTier)
	return orgResponse{
		ID:                  org.ID.String(),
		Name:                org.Name,
		RateLimit:           org.RateLimit,
		Active:              org.Active,
		LogoURL:             org.LogoURL,
		Website:             org.Website,
		PromptCompany:       org.PromptCompany,
		SandboxExposedPorts: sandboxExposedPorts,
		CapacityTier:        limits.Tier,
		ConcurrentSessions:  limits.ConcurrentSessions,
		MaxSandboxSize:      limits.MaxSandboxSize,
		KnowledgeStorageGB:  limits.KnowledgeStorageBytes >> 30,
		CreatedAt:           org.CreatedAt.Format(time.RFC3339),
		OnboardingStep:      org.OnboardingStep,
	}
}

type createOrgRequest struct {
	Name string `json:"name"`
}

type updateOrgRequest struct {
	Name                *string `json:"name,omitempty"`
	LogoURL             *string `json:"logo_url,omitempty"`
	Website             *string `json:"website,omitempty"`
	PromptCompany       *string `json:"prompt_company,omitempty"`
	SandboxExposedPorts *[]int  `json:"sandbox_exposed_ports,omitempty"`
}

type orgResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	RateLimit           int    `json:"rate_limit"`
	Active              bool   `json:"active"`
	LogoURL             string `json:"logo_url,omitempty"`
	Website             string `json:"website,omitempty"`
	PromptCompany       string `json:"prompt_company,omitempty"`
	SandboxExposedPorts []int  `json:"sandbox_exposed_ports"`
	CapacityTier        int    `json:"capacity_tier"`
	ConcurrentSessions  int    `json:"concurrent_session_limit"`
	MaxSandboxSize      string `json:"max_sandbox_size"`
	KnowledgeStorageGB  int64  `json:"knowledge_storage_limit_gb"`
	CreatedAt           string `json:"created_at"`
	OnboardingStep      string `json:"onboarding_step"`
}

// Create handles POST /v1/orgs.
// @Summary Create an organization
// @Description Creates a new organization and adds the requesting user as an admin member.
// @Tags orgs
// @Accept json
// @Produce json
// @Param body body createOrgRequest true "Organization name"
// @Success 201 {object} orgResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs [post]
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := middleware.AuthClaimsFromContext(ctx)
	if !ok || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	var org model.Org
	var membership model.OrgMembership

	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org = model.Org{
			Name:                req.Name,
			SandboxExposedPorts: model.SandboxExposedPortsInt64Array(model.DefaultSandboxExposedPorts()),
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("creating org: %w", err)
		}

		membership = model.OrgMembership{
			UserID: uuid.MustParse(claims.UserID),
			OrgID:  org.ID,
			Role:   "owner",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("creating membership: %w", err)
		}

		// Every org is born with a self-sufficient first team (its own Hivy +
		// #general). Additional orgs are not signup, so there is no user-provided
		// team name; fall back to the default.
		if _, err := provisionFirstTeam(ctx, tx, org.ID, uuid.MustParse(claims.UserID), defaultTeamName); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create org", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}
	enqueueOrgHivyAgentProvision(r.Context(), h.enq, org.ID, "org_create")

	writeJSON(w, http.StatusCreated, h.buildOrgResponse(org))
}

// Current handles GET /v1/orgs/current.
// @Summary Get current organization
// @Description Returns the organization resolved from the request's auth context.
// @Tags orgs
// @Produce json
// @Success 200 {object} orgResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current [get]
func (h *OrgHandler) Current(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	writeJSON(w, http.StatusOK, h.buildOrgResponse(*org))
}

// Update handles PATCH /v1/orgs/current.
// @Summary Update current organization
// @Description Updates workspace profile fields on the current organization.
// @Description Admins and owners only. Pass an empty string for optional fields to clear them.
// @Tags orgs
// @Accept json
// @Produce json
// @Param body body updateOrgRequest true "Fields to patch"
// @Success 200 {object} orgResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current [patch]
func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctxOrg, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	var req updateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == nil && req.LogoURL == nil && req.Website == nil && req.PromptCompany == nil && req.SandboxExposedPorts == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = trimmed
	}
	if req.LogoURL != nil {
		updates["logo_url"] = strings.TrimSpace(*req.LogoURL)
	}
	if req.Website != nil {
		updates["website"] = strings.TrimSpace(*req.Website)
	}
	if req.PromptCompany != nil {
		updates["prompt_company"] = strings.TrimSpace(*req.PromptCompany)
	}
	if req.SandboxExposedPorts != nil {
		ports, err := model.NormalizeExplicitSandboxExposedPorts(*req.SandboxExposedPorts)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		updates["sandbox_exposed_ports"] = model.SandboxExposedPortsInt64Array(ports)
	}

	if err := h.db.Model(&model.Org{}).Where("id = ?", ctxOrg.ID).Updates(updates).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update org", "org_id", ctxOrg.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update organization"})
		return
	}

	var org model.Org
	if err := h.db.First(&org, "id = ?", ctxOrg.ID).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to reload org after update", "org_id", ctxOrg.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload organization"})
		return
	}

	writeJSON(w, http.StatusOK, h.buildOrgResponse(org))
}
