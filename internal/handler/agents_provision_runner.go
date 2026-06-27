package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (h *AgentHandler) EnsureOrgHivyAgent(ctx context.Context, orgID uuid.UUID) error {
	if h == nil || h.db == nil {
		return fmt.Errorf("agent provision not configured")
	}
	_, err := ensureHivyAgent(ctx, h.db, orgID)
	if err != nil {
		return fmt.Errorf("ensure Hivy agent: %w", err)
	}
	return nil
}
