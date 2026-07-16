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
)

func (h *AgentHandler) teamMissingRequiredConnections(ctx context.Context, orgID, teamID uuid.UUID, providers []string) ([]string, error) {
	missing := []string{}
	for _, provider := range providers {
		var count int64
		err := h.db.WithContext(ctx).Table("team_connection_grants tcg").
			Joins("LEFT JOIN connections c ON c.id = tcg.connection_id AND c.revoked_at IS NULL").
			Joins("LEFT JOIN integrations i ON i.id = c.integration_id AND i.deleted_at IS NULL").
			Joins("LEFT JOIN database_connections dc ON dc.id = tcg.database_connection_id AND dc.revoked_at IS NULL").
			Where("tcg.org_id = ? AND tcg.team_id = ? AND (i.provider = ? OR dc.provider = ?)", orgID, teamID, provider, provider).Count(&count).Error
		if err != nil {
			return nil, err
		}
		if count == 0 {
			missing = append(missing, provider)
		}
	}
	return missing, nil
}

func (h *AgentHandler) resolveCatalogInstallTeam(ctx context.Context, w http.ResponseWriter, r *http.Request, orgID uuid.UUID) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("team_id"))
	if raw == "" && r.Body != nil {
		var body agentCatalogInstallRequest
		if json.NewDecoder(r.Body).Decode(&body) == nil {
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

func activeAgentForCatalogTeam(ctx context.Context, tx *gorm.DB, orgID, catalogID, teamID uuid.UUID) (model.Agent, bool, error) {
	var agent model.Agent
	err := tx.WithContext(ctx).Where("org_id = ? AND agent_catalog_id = ? AND team_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, catalogID, teamID, "archived").First(&agent).Error
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
	desc, avatar, catalogID := catalog.Description, catalog.AvatarURL, catalog.ID
	agent := model.Agent{OrgID: &orgID, TeamID: teamID, AgentCatalogID: &catalogID, InstructionsSnapshot: snapshotCatalogInstructions(catalog.Instructions), Name: catalog.Name, Description: &desc, AvatarURL: optionalStringPtr(avatar), SandboxImage: model.NormalizeSandboxImage(catalog.SandboxImage), SandboxSize: model.DefaultAgentSandboxSize, Model: modelID, DefaultReasoningEffort: catalog.DefaultReasoningEffort, AutoLoadSkills: append(model.AutoLoadSkills(nil), catalog.AutoLoadSkills...), Tools: cloneModelJSON(catalog.Tools), McpServers: model.RawJSON("[]"), Skills: model.JSON{}, Permissions: permissions, Resources: model.JSON{}, SandboxTools: pq.StringArray(sandboxTools), Status: "active"}
	if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return model.Agent{}, fmt.Errorf("create catalog agent: %w", err)
	}
	return agent, nil
}
