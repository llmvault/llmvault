package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
	"github.com/usehivy/hivy/internal/teamprovision"
)

// teamMissingRequiredPlugins returns the catalog's required-plugin slugs that
// the team cannot use. A plugin is usable by a team when it is an auto-install
// system plugin, or the org has installed it AND it is provisioned for the team.
func (h *AgentHandler) teamMissingRequiredPlugins(ctx context.Context, orgID, teamID uuid.UUID, slugs []string) ([]string, error) {
	missing := []string{}
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}
		usable, err := h.pluginUsableByTeam(ctx, orgID, teamID, slug)
		if err != nil {
			return nil, err
		}
		if !usable {
			missing = append(missing, slug)
		}
	}
	return missing, nil
}

func (h *AgentHandler) pluginUsableByTeam(ctx context.Context, orgID, teamID uuid.UUID, slug string) (bool, error) {
	var plugin model.Plugin
	err := h.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, model.PluginStatusActive).
		First(&plugin).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if pluginstore.PluginAutoInstall(plugin) {
		return true, nil
	}
	installed, err := h.orgPluginInstalled(ctx, orgID, plugin.ID)
	if err != nil {
		return false, err
	}
	if !installed {
		return false, nil
	}
	return teamprovision.PluginEnabledForTeam(ctx, h.db, teamID, plugin.ID)
}

// resolveCatalogInstallTeam resolves and authorizes the target team for a
// catalog install/uninstall. The team_id may arrive in the JSON body or the
// query string. Installing/uninstalling a catalog agent is a team-scoped member
// action: the actor must be able to manage the team, and a team_id is always
// required (there is no org-level catalog install anymore). Writes an error
// response and returns ok=false when not allowed.
func (h *AgentHandler) resolveCatalogInstallTeam(ctx context.Context, w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("team_id"))
	if raw == "" && r.Body != nil {
		var body agentCatalogInstallRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			raw = cleanStringPtr(body.TeamID)
		}
	}
	teamID, ok := h.resolveAndAuthorizeAgentTeam(ctx, w, orgID, &raw)
	if !ok {
		return nil, false
	}
	if teamID == nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "team_id is required"})
		return nil, false
	}
	return teamID, true
}

// activeAgentForCatalogTeam finds the team's live clone of a catalog agent.
// Catalog agents are installed once PER TEAM (each team gets its own clone), so
// the "already installed" lookup is scoped by (org, catalog, team).
func activeAgentForCatalogTeam(ctx context.Context, tx *gorm.DB, orgID, catalogID, teamID uuid.UUID) (model.Agent, bool, error) {
	var agent model.Agent
	err := tx.WithContext(ctx).
		Where("org_id = ? AND agent_catalog_id = ? AND team_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, catalogID, teamID, "archived").
		First(&agent).Error
	if err == nil {
		return agent, true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return model.Agent{}, false, nil
	}
	return model.Agent{}, false, err
}

func (h *AgentHandler) createCatalogAgent(ctx context.Context, tx *gorm.DB, orgID, teamID uuid.UUID, catalog model.AgentCatalog) (model.Agent, error) {
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
	teamRef := teamID
	// Instructions is intentionally left nil on the clone (a later user edit that
	// sets it forks the clone). InstructionsSnapshot freezes the catalog template
	// prompt at install: an un-forked clone resolves its prompt
	// from this snapshot, so a subsequent catalog rename/edit/archive cannot
	// silently rewrite or blank it.
	promptSnapshot := snapshotCatalogInstructions(catalog.Instructions)
	agent := model.Agent{
		OrgID:                  &orgID,
		TeamID:                 &teamRef,
		AgentCatalogID:         &catalogID,
		InstructionsSnapshot:   promptSnapshot,
		Name:                   catalog.Name,
		Description:            &desc,
		AvatarURL:              optionalStringPtr(avatarURL),
		IsDefault:              false,
		SandboxImage:           model.NormalizeSandboxImage(catalog.SandboxImage),
		SandboxSize:            model.DefaultAgentSandboxSize,
		Model:                  modelID,
		DefaultReasoningEffort: strings.TrimSpace(catalog.DefaultReasoningEffort),
		AutoLoadSkills:         append(model.AutoLoadSkills(nil), catalog.AutoLoadSkills...),
		Tools:                  cloneModelJSON(catalog.Tools),
		McpServers:             model.RawJSON("[]"),
		Skills:                 model.JSON{},
		Permissions:            permissions,
		Resources:              model.JSON{},
		SandboxTools:           pq.StringArray(sandboxTools),
		Status:                 "active",
	}
	agent.Name = catalog.Name
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
