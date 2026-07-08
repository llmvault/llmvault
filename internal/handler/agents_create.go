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
	pluginstore "github.com/usehivy/hivy/internal/plugins"
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
		agent, err := createHivyAgentWithDefaultsTx(ctx, tx, orgID, nil)
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

// createHivyAgentWithDefaultsTx creates the default Hivy agent. teamID is the
// team the Hivy belongs to (each team gets its own undeletable Hivy clone); it
// is nil only for the legacy org-level fallback (ensureHivyAgent), which
// creates a single team-less default agent.
func createHivyAgentWithDefaultsTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID, teamID *uuid.UUID) (*model.Agent, error) {
	return createHivyAgentTx(ctx, tx, orgID, teamID)
}

func createHivyAgentTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID, teamID *uuid.UUID) (*model.Agent, error) {
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
		// Freeze the catalog template prompt at creation so a later
		// catalog edit/archive cannot silently rewrite or blank this Hivy clone.
		agent.InstructionsSnapshot = snapshotCatalogInstructions(catalog.Instructions)
	}
	if teamID != nil {
		// Each team owns its own Hivy clone, so several "Hivy" agents can coexist
		// in one org. Uniquify the name (Hivy, Hivy-2, ...) to avoid colliding on
		// idx_agents_org_name.
		if err := createWithUniqueNameSlug(tx.WithContext(ctx), &agent, name); err != nil {
			return nil, fmt.Errorf("create Hivy agent: %w", err)
		}
	} else if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, fmt.Errorf("create Hivy agent: %w", err)
	}
	if err := pluginstore.EnsureAutoInstalledForAgent(ctx, tx, orgID, agent.ID); err != nil {
		return nil, err
	}
	// The default Hivy agent also gets plugins flagged "default_agent_install"
	// (e.g. the agent-builder capability), which are not installed on other agents.
	if err := pluginstore.EnsureDefaultAgentPluginsForAgent(ctx, tx, orgID, agent.ID); err != nil {
		return nil, err
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

const agentSlugMaxAttempts = 32

func slugifyAgentName(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func createWithUniqueNameSlug(tx *gorm.DB, agent *model.Agent, baseSlug string) error {
	for i := 0; i < agentSlugMaxAttempts; i++ {
		candidate := baseSlug
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", baseSlug, i+1)
		}
		agent.Name = candidate
		agent.ID = uuid.Nil

		exists, err := agentNameExists(tx, agent.OrgID, candidate)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		sp := fmt.Sprintf("sp_agent_slug_attempt_%d", i)
		if err := tx.SavePoint(sp).Error; err != nil {
			return fmt.Errorf("savepoint: %w", err)
		}
		err = tx.Create(agent).Error
		if err == nil {
			return nil
		}
		if !isDuplicateKeyError(err) {
			return err
		}
		if rbErr := tx.RollbackTo(sp).Error; rbErr != nil {
			return fmt.Errorf("rollback to savepoint: %w", rbErr)
		}
	}
	return fmt.Errorf("could not allocate unique agent name after %d attempts (base=%s)", agentSlugMaxAttempts, baseSlug)
}

func agentNameExists(tx *gorm.DB, orgID *uuid.UUID, name string) (bool, error) {
	var count int64
	query := tx.Model(&model.Agent{}).Where("name = ?", name)
	if orgID == nil {
		query = query.Where("org_id IS NULL")
	} else {
		query = query.Where("org_id = ?", *orgID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check agent name: %w", err)
	}
	return count > 0, nil
}
