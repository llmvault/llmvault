package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func (h *SlackAppMentionHandler) ensureOrgAgent(ctx context.Context, orgID uuid.UUID) (model.Agent, error) {
	agent, err := h.loadFirstOrgAgent(ctx, orgID)
	if err == nil {
		return agent, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Agent{}, err
	}
	if h.orgAgentEnsurer == nil {
		return model.Agent{}, fmt.Errorf("org Hivy agent ensurer is not configured")
	}
	if err := h.orgAgentEnsurer.EnsureOrgHivyAgent(ctx, orgID); err != nil {
		return model.Agent{}, fmt.Errorf("ensure org Hivy agent: %w", err)
	}
	return h.loadFirstOrgAgent(ctx, orgID)
}

func (h *SlackAppMentionHandler) loadFirstOrgAgent(ctx context.Context, orgID uuid.UUID) (model.Agent, error) {
	var agent model.Agent
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND status <> ?", orgID, "archived").
		Order("created_at ASC").
		First(&agent).Error
	if err != nil {
		return model.Agent{}, fmt.Errorf("load org agent: %w", err)
	}
	return agent, nil
}

func (h *SlackAppMentionHandler) loadAgent(ctx context.Context, orgID, agentID uuid.UUID) (model.Agent, error) {
	var agent model.Agent
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error
	if err != nil {
		return model.Agent{}, fmt.Errorf("load channel agent: %w", err)
	}
	return agent, nil
}
