package tasks

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

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
