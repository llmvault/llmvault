package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func ensureHivyAgent(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*model.Agent, error) {
	var existing model.Agent
	err := db.WithContext(ctx).
		Where("org_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, "archived").
		Order("created_at ASC").
		First(&existing).Error
	if err == nil {
		if err := applyHivyAgentRuntimeDefaults(ctx, db, &existing); err != nil {
			return nil, err
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup Hivy agent: %w", err)
	}

	var out *model.Agent
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		teamID, err := orgOldestTeamIDTx(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if teamID == uuid.Nil {
			team, err := provisionFirstTeam(ctx, tx, orgID, uuid.Nil, "")
			if err != nil {
				return err
			}
			teamID = team.ID
		}
		agent, err := ensureTeamHivyTx(ctx, tx, orgID, teamID)
		if err != nil {
			return err
		}
		out = agent
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			if refetch := db.WithContext(ctx).
				Where("org_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, "archived").
				Order("created_at ASC").
				First(&existing).Error; refetch == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	return out, nil
}

// orgOldestTeamIDTx returns the org's oldest team, or uuid.Nil when the org has
// no team yet.
func orgOldestTeamIDTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (uuid.UUID, error) {
	var team model.Team
	err := tx.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("created_at ASC").
		First(&team).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup oldest team: %w", err)
	}
	return team.ID, nil
}

// ensureTeamHivyTx returns the team's existing Hivy agent or creates it.
func ensureTeamHivyTx(ctx context.Context, tx *gorm.DB, orgID, teamID uuid.UUID) (*model.Agent, error) {
	var existing model.Agent
	err := tx.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND status <> ? AND parent_agent_id IS NULL", orgID, teamID, "archived").
		Order("created_at ASC").
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup team Hivy agent: %w", err)
	}
	return createHivyAgentWithDefaultsTx(ctx, tx, orgID, teamID)
}

// createHivyAgentWithDefaultsTx creates a team's default Hivy agent. teamID is
// the team the Hivy belongs to (each team gets its own undeletable Hivy clone).
func createHivyAgentWithDefaultsTx(ctx context.Context, tx *gorm.DB, orgID, teamID uuid.UUID) (*model.Agent, error) {
	return createHivyAgentTx(ctx, tx, orgID, teamID)
}

func createHivyAgentTx(ctx context.Context, tx *gorm.DB, orgID, teamID uuid.UUID) (*model.Agent, error) {
	catalog, hasCatalog, err := loadDefaultAgentCatalog(ctx, tx)
	if err != nil {
		return nil, err
	}
	modelID := agentruntime.DefaultAgentModel
	if hasCatalog && strings.TrimSpace(catalog.Model) != "" {
		modelID = strings.TrimSpace(catalog.Model)
	}
	if err := registry.Global().ValidateCanonicalModel(modelID); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "default Hivy catalog model is not canonical", "error", err, "org_id", orgID, "model", modelID)
		modelID = agentruntime.DefaultAgentModel
	}

	name := hivyAgentName
	desc := hivyAgentDescription
	avatarURL := hivyAgentAvatarURL
	sandboxImage := model.SandboxImageDefault
	var catalogID *uuid.UUID
	if hasCatalog {
		catalogID = &catalog.ID
		name = catalog.Name
		desc = catalog.Description
		if strings.TrimSpace(catalog.AvatarURL) != "" {
			avatarURL = catalog.AvatarURL
		}
		if model.ValidSandboxImage(catalog.SandboxImage) {
			sandboxImage = model.NormalizeSandboxImage(catalog.SandboxImage)
		}
	}
	tools := model.JSON{}
	if hasCatalog {
		tools = cloneModelJSON(catalog.Tools)
	}
	agent := model.Agent{
		OrgID:                  &orgID,
		TeamID:                 teamID,
		AgentCatalogID:         catalogID,
		Name:                   name,
		Description:            &desc,
		AvatarURL:              ptrString(avatarURL),
		IsDefault:              true,
		SandboxImage:           sandboxImage,
		SandboxSize:            model.DefaultHivyAgentSandboxSize,
		Model:                  modelID,
		DefaultReasoningEffort: strings.TrimSpace(catalog.DefaultReasoningEffort),
		AutoLoadSkills:         append(model.AutoLoadSkills(nil), catalog.AutoLoadSkills...),
		Status:                 "active",
		Tools:                  tools,
		McpServers:             model.RawJSON("[]"),
		Skills:                 model.JSON{},
		Integrations:           model.JSON{},
		Resources:              model.JSON{},
		RuntimeConfig:          model.JSON{},
		Permissions:            model.JSON{},
	}
	if hasCatalog {
		// Seed a fallback snapshot; nil Instructions keeps the live catalog prompt
		// authoritative until the user provides a prompt override.
		agent.InstructionsSnapshot = snapshotCatalogInstructions(catalog.Instructions)
	}
	agent.Name = name
	if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, fmt.Errorf("create Hivy agent: %w", err)
	}

	return &agent, nil
}

func applyHivyAgentRuntimeDefaults(ctx context.Context, db *gorm.DB, agent *model.Agent) error {
	if agent == nil || !agent.IsDefault {
		return nil
	}
	updates := map[string]any{}
	if model.NormalizeTemplateSize(agent.SandboxSize) != model.DefaultHivyAgentSandboxSize {
		updates["sandbox_size"] = model.DefaultHivyAgentSandboxSize
		agent.SandboxSize = model.DefaultHivyAgentSandboxSize
	}
	tools := cloneModelJSON(agent.Tools)
	toolsChanged := false
	for _, id := range []string{"write_file", "apply_patch"} {
		if enabled, ok := tools[id].(bool); !ok || !enabled {
			tools[id] = true
			toolsChanged = true
		}
	}
	if toolsChanged {
		updates["tools"] = tools
		agent.Tools = tools
	}
	if len(updates) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).Model(&model.Agent{}).
		Where("id = ? AND status <> ?", agent.ID, "archived").
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update Hivy agent runtime defaults: %w", err)
	}
	return nil
}

func loadDefaultAgentCatalog(ctx context.Context, tx *gorm.DB) (model.AgentCatalog, bool, error) {
	var catalog model.AgentCatalog
	err := tx.WithContext(ctx).
		Where("slug = ? AND status = ?", "hivy", model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err == nil {
		return catalog, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AgentCatalog{}, false, fmt.Errorf("load default agent catalog: %w", err)
	}
	err = tx.WithContext(ctx).
		Where("is_default = ? AND status = ?", true, model.AgentCatalogStatusActive).
		Order("created_at ASC").
		First(&catalog).Error
	if err == nil {
		return catalog, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AgentCatalog{}, false, nil
	}
	return model.AgentCatalog{}, false, fmt.Errorf("load default agent catalog: %w", err)
}

func ptrString(value string) *string {
	return &value
}

func (h *AgentHandler) rollbackAgent(ctx context.Context, orgID, agentID, companionAgentID uuid.UUID) {
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND agent_id = ?", orgID, agentID).Delete(&model.Sandbox{}).Error; err != nil {
			return fmt.Errorf("delete sandbox: %w", err)
		}
		if err := tx.Where("org_id = ? AND id = ?", orgID, agentID).Delete(&model.Agent{}).Error; err != nil {
			return fmt.Errorf("delete agent agent: %w", err)
		}
		if companionAgentID != uuid.Nil {
			if err := tx.Where("org_id = ? AND id = ?", orgID, companionAgentID).Delete(&model.Agent{}).Error; err != nil {
				return fmt.Errorf("delete companion agent: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "rollback agent", "error", err,
			"agent_id", agentID, "companion_agent_id", companionAgentID, "org_id", orgID)
	}
}
