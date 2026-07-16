package connectionaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
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

	query := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN team_connection_grants tcg ON tcg.connection_id = connections.id AND tcg.org_id = connections.org_id").
		Where("tcg.team_id = ?", agent.TeamID).
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, provider)
	if disabled := agent.ConnectionMCPToolDeny.DisabledConnectionIDs(); len(disabled) > 0 {
		query = query.Where("connections.id NOT IN ?", disabled)
	}
	var conn model.Connection
	err := query.Order("connections.created_at ASC").First(&conn).Error
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
	if agent.ConnectionMCPToolDeny.DisablesConnection(connectionID) {
		return Result{}, gorm.ErrRecordNotFound
	}

	var conn model.Connection
	err := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN team_connection_grants tcg ON tcg.connection_id = connections.id AND tcg.org_id = connections.org_id").
		Where("tcg.team_id = ?", agent.TeamID).
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

// IntegrationConnections returns the active managed connections granted to an
// agent's team.
func IntegrationConnections(ctx context.Context, db *gorm.DB, agent model.Agent) ([]model.Connection, error) {
	if db == nil || agent.OrgID == nil || agent.TeamID == uuid.Nil {
		return []model.Connection{}, nil
	}
	query := db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN team_connection_grants tcg ON tcg.connection_id = connections.id AND tcg.org_id = connections.org_id").
		Where("tcg.team_id = ? AND connections.org_id = ? AND connections.revoked_at IS NULL", agent.TeamID, *agent.OrgID)
	if disabled := agent.ConnectionMCPToolDeny.DisabledConnectionIDs(); len(disabled) > 0 {
		query = query.Where("connections.id NOT IN ?", disabled)
	}
	var rows []model.Connection
	err := query.Order("connections.created_at ASC, connections.id ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load granted integration connections: %w", err)
	}
	return rows, nil
}

// DatabaseConnections returns the active database connections granted to an
// agent's team.
func DatabaseConnections(ctx context.Context, db *gorm.DB, agent model.Agent) ([]model.DatabaseConnection, error) {
	if db == nil || agent.OrgID == nil || agent.TeamID == uuid.Nil {
		return []model.DatabaseConnection{}, nil
	}
	query := db.WithContext(ctx).
		Joins("JOIN team_connection_grants tcg ON tcg.database_connection_id = database_connections.id AND tcg.org_id = database_connections.org_id").
		Where("tcg.team_id = ? AND database_connections.org_id = ? AND database_connections.revoked_at IS NULL", agent.TeamID, *agent.OrgID)
	if disabled := agent.ConnectionMCPToolDeny.DisabledConnectionIDs(); len(disabled) > 0 {
		query = query.Where("database_connections.id NOT IN ?", disabled)
	}
	var rows []model.DatabaseConnection
	err := query.Order("database_connections.created_at ASC, database_connections.id ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load granted database connections: %w", err)
	}
	return rows, nil
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
