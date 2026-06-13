package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentprompts"
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func ensureHivyAgent(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*model.Agent, error) {
	var existing model.Agent
	err := db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Order("created_at ASC").
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup Hivy agent: %w", err)
	}

	var out *model.Agent
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		agent, err := createHivyAgentWithDefaultsTx(ctx, tx, orgID)
		if err != nil {
			return err
		}
		out = agent
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			if refetch := db.WithContext(ctx).
				Where("org_id = ? AND status <> ?", orgID, "archived").
				Order("created_at ASC").
				First(&existing).Error; refetch == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	return out, nil
}

func createHivyAgentWithDefaultsTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (*model.Agent, error) {
	agent, err := createHivyAgentTx(ctx, tx, orgID)
	if err != nil {
		return nil, err
	}
	if err := attachPublishedGlobalSkillsTx(ctx, tx, agent.ID, defaultAgentSkills); err != nil {
		return nil, err
	}
	return agent, nil
}

func createHivyAgentTx(ctx context.Context, tx *gorm.DB, orgID uuid.UUID) (*model.Agent, error) {
	choice, err := pickAgentCredential(tx)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "no provider credential available for Hivy agent", "error", err, "org_id", orgID)
		choice = &agentProviderChoice{model: agentruntime.DefaultAgentModel}
	}

	desc := hivyAgentDescription
	agent := model.Agent{
		OrgID:           &orgID,
		Name:            hivyAgentName,
		Description:     &desc,
		AvatarURL:       ptrString(hivyAgentAvatarURL),
		IsDefault:       true,
		SandboxStrategy: agentStrategyAlwaysOn,
		SystemPrompt:    "",
		IdentityPrompt:  agentprompts.EngineeringIdentityPrompt,
		Model:           choice.model,
		Harness:         agentHarness,
		Status:          "active",
		SharedMemory:    true,
		Tools:           model.JSON{},
		McpServers:      model.RawJSON("[]"),
		Skills:          model.JSON{},
		Integrations:    model.JSON{},
		Resources:       model.JSON{},
		RuntimeConfig:   model.JSON{},
		Permissions:     model.JSON{},
	}
	if choice.cred != nil {
		agent.CredentialID = &choice.cred.ID
	}
	if err := tx.WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, fmt.Errorf("create Hivy agent: %w", err)
	}

	return &agent, nil
}

func ptrString(value string) *string {
	return &value
}

func attachPublishedGlobalSkillsTx(ctx context.Context, tx *gorm.DB, agentID uuid.UUID, names []string) error {
	required := make(map[string]bool, len(names))
	for _, name := range names {
		required[name] = true
	}
	skills, err := loadPublishedGlobalSkillsByName(ctx, tx, required)
	if err != nil {
		return err
	}
	for _, name := range names {
		skill, ok := skills[name]
		if !ok {
			logging.FromContext(ctx).WarnContext(ctx, "global skill not attached to Hivy", "skill_name", name, "agent_id", agentID)
			continue
		}
		link := model.AgentSkill{AgentID: agentID, SkillID: skill.ID}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return fmt.Errorf("attach global skill %q: %w", name, err)
		}
	}
	return nil
}

func (h *AgentHandler) attachGlobalSkills(ctx context.Context, agentID uuid.UUID, names []string) {
	attachPublishedGlobalSkills(ctx, h.db, agentID, names)
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
