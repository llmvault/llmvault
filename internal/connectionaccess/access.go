package connectionaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

type Result struct {
	Agent             model.Agent
	Connection        model.Connection
	ProviderConfigKey string
	Resources         model.JSON
}

func ResolveAgentProvider(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, provider string) (Result, error) {
	if db == nil {
		return Result{}, fmt.Errorf("db is required")
	}
	if orgID == uuid.Nil || agentID == uuid.Nil || provider == "" {
		return Result{}, gorm.ErrRecordNotFound
	}

	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return Result{}, err
	}

	pluginIDs, err := pluginresolve.EffectivePluginIDs(ctx, db, agent)
	if err != nil {
		return Result{}, err
	}
	if len(pluginIDs) == 0 {
		return Result{}, gorm.ErrRecordNotFound
	}

	var conn model.Connection
	err = db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN plugin_integrations ON plugin_integrations.provider = integrations.provider AND plugin_integrations.kind = ?", model.PluginIntegrationKindIntegration).
		Where("plugin_integrations.plugin_id IN ?", pluginIDs).
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, provider).
		Order("connections.created_at ASC").
		First(&conn).Error
	if err != nil {
		return Result{}, err
	}
	if conn.NangoConnectionID == "" {
		return Result{}, fmt.Errorf("%s connection missing nango connection id", provider)
	}

	return Result{
		Agent:             agent,
		Connection:        conn,
		ProviderConfigKey: NangoProviderConfigKey(conn.Integration.UniqueKey),
		Resources:         EffectiveResources(agent.Resources, conn),
	}, nil
}

func ResolveAgentProviderAny(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, providers ...string) (Result, error) {
	var lastErr error = gorm.ErrRecordNotFound
	for _, provider := range providers {
		result, err := ResolveAgentProvider(ctx, db, orgID, agentID, provider)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return Result{}, err
		}
	}
	return Result{}, lastErr
}

func ResolveAgentConnection(ctx context.Context, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, connectionID uuid.UUID) (Result, error) {
	if db == nil {
		return Result{}, fmt.Errorf("db is required")
	}
	if orgID == uuid.Nil || agentID == uuid.Nil || connectionID == uuid.Nil {
		return Result{}, gorm.ErrRecordNotFound
	}

	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return Result{}, err
	}

	pluginIDs, err := pluginresolve.EffectivePluginIDs(ctx, db, agent)
	if err != nil {
		return Result{}, err
	}
	if len(pluginIDs) == 0 {
		return Result{}, gorm.ErrRecordNotFound
	}

	var conn model.Connection
	err = db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN plugin_integrations ON plugin_integrations.provider = integrations.provider AND plugin_integrations.kind = ?", model.PluginIntegrationKindIntegration).
		Where("plugin_integrations.plugin_id IN ?", pluginIDs).
		Where("connections.id = ? AND connections.org_id = ? AND connections.revoked_at IS NULL", connectionID, orgID).
		First(&conn).Error
	if err != nil {
		return Result{}, err
	}
	if conn.NangoConnectionID == "" {
		return Result{}, fmt.Errorf("%s connection missing nango connection id", conn.Integration.Provider)
	}
	return Result{
		Agent:             agent,
		Connection:        conn,
		ProviderConfigKey: NangoProviderConfigKey(conn.Integration.UniqueKey),
		Resources:         EffectiveResources(agent.Resources, conn),
	}, nil
}

func EffectiveResources(agentResources model.JSON, conn model.Connection) model.JSON {
	if resources := resourcesForConnection(agentResources, conn.ID.String()); resources != nil {
		return resources
	}
	if resources := resourcesFromConnectionMeta(conn.Meta); resources != nil {
		return resources
	}
	return model.JSON{}
}

func resourcesForConnection(resources model.JSON, connectionID string) model.JSON {
	if len(resources) == 0 || connectionID == "" {
		return nil
	}
	raw, ok := resources[connectionID]
	if !ok {
		return nil
	}
	return jsonObject(raw)
}

func resourcesFromConnectionMeta(meta model.JSON) model.JSON {
	if len(meta) == 0 {
		return nil
	}
	raw, ok := meta["resources"]
	if !ok {
		return nil
	}
	return jsonObject(raw)
}

func jsonObject(raw any) model.JSON {
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case model.JSON:
		out := model.JSON{}
		for key, value := range typed {
			out[key] = value
		}
		return out
	case map[string]any:
		out := model.JSON{}
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		return nil
	}
}

func NangoProviderConfigKey(uniqueKey string) string {
	if uniqueKey == "" {
		return ""
	}
	return uniqueKey
}

func IsGitHubProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	return provider == "github-app" || provider == "github-app-code-reviews"
}
