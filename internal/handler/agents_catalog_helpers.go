package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

func (h *AgentHandler) toAgentCatalogResponse(ctx context.Context, orgID uuid.UUID, c model.AgentCatalog) (agentCatalogResponse, error) {
	required, err := h.catalogPluginSummaries(ctx, orgID, c.RequiredPlugins)
	if err != nil {
		return agentCatalogResponse{}, err
	}
	recommended, err := h.catalogPluginSummaries(ctx, orgID, c.RecommendedPlugins)
	if err != nil {
		return agentCatalogResponse{}, err
	}
	var installedID *string
	var agent model.Agent
	err = h.db.WithContext(ctx).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, c.ID, "archived").
		First(&agent).Error
	if err == nil {
		value := agent.ID.String()
		installedID = &value
	} else if err != gorm.ErrRecordNotFound {
		return agentCatalogResponse{}, err
	}
	return agentCatalogResponse{
		ID:                 c.ID.String(),
		Slug:               c.Slug,
		Name:               c.Name,
		Description:        c.Description,
		Category:           c.Category,
		AvatarURL:          c.AvatarURL,
		Developer:          c.Developer,
		Official:           c.Official,
		IsDefault:          c.IsDefault,
		Model:              c.Model,
		SandboxImage:       model.NormalizeSandboxImage(c.SandboxImage),
		RequiredPlugins:    required,
		RecommendedPlugins: recommended,
		InstalledAgentID:   installedID,
	}, nil
}

func (h *AgentHandler) catalogPluginSummaries(ctx context.Context, orgID uuid.UUID, slugs []string) ([]agentCatalogPluginSummary, error) {
	out := make([]agentCatalogPluginSummary, 0, len(slugs))
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}
		summary, err := h.catalogPluginSummary(ctx, orgID, slug)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

func (h *AgentHandler) catalogPluginSummary(ctx context.Context, orgID uuid.UUID, slug string) (agentCatalogPluginSummary, error) {
	var plugin model.Plugin
	err := h.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, model.PluginStatusActive).
		First(&plugin).Error
	if err == gorm.ErrRecordNotFound {
		return agentCatalogPluginSummary{Slug: slug, Name: slug}, nil
	}
	if err != nil {
		return agentCatalogPluginSummary{}, err
	}
	installed, err := h.orgPluginInstalled(ctx, orgID, plugin.ID)
	if err != nil {
		return agentCatalogPluginSummary{}, err
	}
	return agentCatalogPluginSummary{
		ID:        plugin.ID.String(),
		Slug:      plugin.Slug,
		Name:      plugin.Name,
		Installed: installed,
	}, nil
}

func (h *AgentHandler) missingInstalledPlugins(ctx context.Context, orgID uuid.UUID, slugs []string) ([]agentCatalogPluginSummary, error) {
	var missing []agentCatalogPluginSummary
	for _, slug := range slugs {
		summary, err := h.catalogPluginSummary(ctx, orgID, slug)
		if err != nil {
			return nil, err
		}
		if !summary.Installed {
			missing = append(missing, summary)
		}
	}
	return missing, nil
}

func (h *AgentHandler) orgPluginInstalled(ctx context.Context, orgID, pluginID uuid.UUID) (bool, error) {
	var count int64
	err := h.db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", orgID, pluginID).
		Count(&count).Error
	return count > 0, err
}

func (h *AgentHandler) loadAgentCatalogBySlug(w http.ResponseWriter, r *http.Request, slug string) (model.AgentCatalog, bool) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent catalog slug required"})
		return model.AgentCatalog{}, false
	}
	var catalog model.AgentCatalog
	err := h.db.WithContext(r.Context()).
		Where("slug = ? AND status = ?", slug, model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err == nil {
		return catalog, true
	}
	if err == gorm.ErrRecordNotFound {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent catalog entry not found"})
		return model.AgentCatalog{}, false
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent catalog entry"})
	return model.AgentCatalog{}, false
}

func activeAgentForCatalog(ctx context.Context, tx *gorm.DB, orgID, catalogID uuid.UUID) (model.Agent, bool, error) {
	var agent model.Agent
	err := tx.WithContext(ctx).
		Where("org_id = ? AND agent_catalog_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, catalogID, "archived").
		First(&agent).Error
	if err == nil {
		return agent, true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return model.Agent{}, false, nil
	}
	return model.Agent{}, false, err
}

func (h *AgentHandler) createCatalogAgent(ctx context.Context, tx *gorm.DB, orgID uuid.UUID, catalog model.AgentCatalog) (model.Agent, error) {
	modelID := strings.TrimSpace(catalog.Model)
	if modelID == "" {
		modelID = agentruntime.DefaultAgentModel
	}
	if err := h.validateAgentSelectableModel(ctx, orgID, modelID); err != nil {
		return model.Agent{}, err
	}
	permissions := model.JSON{}
	for _, id := range model.BuiltInToolIDs() {
		permissions[id] = true
	}
	sandboxTools := make([]string, 0, len(model.ValidSandboxTools))
	for _, tool := range model.ValidSandboxTools {
		sandboxTools = append(sandboxTools, tool.ID)
	}
	desc := catalog.Description
	avatarURL := catalog.AvatarURL
	catalogID := catalog.ID
	agent := model.Agent{
		OrgID:                  &orgID,
		AgentCatalogID:         &catalogID,
		Name:                   catalog.Name,
		Description:            &desc,
		AvatarURL:              optionalStringPtr(avatarURL),
		IsDefault:              false,
		SandboxImage:           model.NormalizeSandboxImage(catalog.SandboxImage),
		SandboxSize:            model.DefaultAgentSandboxSize,
		Model:                  modelID,
		DefaultReasoningEffort: strings.TrimSpace(catalog.DefaultReasoningEffort),
		Tools:                  cloneModelJSON(catalog.Tools),
		McpServers:             model.RawJSON("[]"),
		Skills:                 model.JSON{},
		Permissions:            permissions,
		Resources:              model.JSON{},
		SandboxTools:           pq.StringArray(sandboxTools),
		Status:                 "active",
	}
	if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return model.Agent{}, fmt.Errorf("create catalog agent: %w", err)
	}
	if err := pluginstore.EnsureAutoInstalledForAgent(ctx, tx, orgID, agent.ID); err != nil {
		return model.Agent{}, err
	}
	return agent, nil
}

func enableRequiredCatalogPlugins(ctx context.Context, tx *gorm.DB, orgID, agentID uuid.UUID, slugs []string) error {
	for _, slug := range slugs {
		var plugin model.Plugin
		err := tx.WithContext(ctx).
			Where("slug = ? AND status = ?", slug, model.PluginStatusActive).
			First(&plugin).Error
		if err != nil {
			return fmt.Errorf("load required plugin %q: %w", slug, err)
		}
		if err := enablePluginForAgent(ctx, tx, orgID, agentID, plugin.ID); err != nil {
			return err
		}
	}
	return nil
}
