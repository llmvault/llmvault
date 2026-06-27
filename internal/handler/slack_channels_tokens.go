package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackChannelHandler) loadSlackBotToken(ctx context.Context, orgID uuid.UUID) (string, error) {
	var conn model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, slackapp.Provider).
		Order("connections.created_at ASC").
		First(&conn).Error; err != nil {
		return "", fmt.Errorf("active Slack connection required: %w", err)
	}
	return h.botTokenFromConnection(ctx, conn)
}

func (h *SlackChannelHandler) loadSlackBotTokenForConnection(ctx context.Context, orgID, connectionID uuid.UUID) (string, error) {
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

func (h *SlackChannelHandler) botTokenFromConnection(ctx context.Context, conn model.Connection) (string, error) {
	providerConfigKey := nangoProviderConfigKey(conn.Integration.UniqueKey)
	nangoConn, err := h.nango.GetConnection(ctx, conn.NangoConnectionID, providerConfigKey)
	if err != nil {
		return "", fmt.Errorf("load Slack connection credentials: %w", err)
	}
	creds, _ := nangoConn["credentials"].(map[string]any)
	for _, key := range []string{"bot_token", "access_token"} {
		if token, _ := creds[key].(string); strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", fmt.Errorf("Slack connection credentials do not include a bot token")
}
