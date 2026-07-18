package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackResourceRouteValidator) loadSlackBotTokenForConnection(ctx context.Context, orgID, connectionID uuid.UUID) (string, error) {
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.id = ? AND connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", connectionID, orgID, slackapp.Provider).
		First(&conn).Error; err != nil {
		return "", fmt.Errorf("active Slack connection required: %w", err)
	}
	return h.botTokenFromConnection(ctx, conn)
}

func (h *SlackResourceRouteValidator) botTokenFromConnection(ctx context.Context, conn model.Connection) (string, error) {
	return slackapp.LoadBotToken(ctx, h.nango, conn)
}
