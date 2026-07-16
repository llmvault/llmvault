package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type ServiceProxyEnvSpec struct {
	Provider   string
	BaseURLEnv string
	AuthEnv    string
	Path       string
}

var serviceProxyEnvSpecs = []ServiceProxyEnvSpec{
	{Provider: "postgres", BaseURLEnv: AgentEnvPostgresURL, AuthEnv: AgentEnvPostgresToken, Path: "/internal/database-proxy/postgres/%s"},
	{Provider: "mysql", BaseURLEnv: AgentEnvMySQLURL, AuthEnv: AgentEnvMySQLToken, Path: "/internal/database-proxy/mysql/%s"},
	{Provider: "mongodb", BaseURLEnv: AgentEnvMongoDBURL, AuthEnv: AgentEnvMongoDBToken, Path: "/internal/database-proxy/mongodb/%s"},
	{Provider: "redis", BaseURLEnv: AgentEnvRedisURL, AuthEnv: AgentEnvRedisToken, Path: "/internal/database-proxy/redis/%s"},
}

func ServiceProxyEnvSpecs() []ServiceProxyEnvSpec {
	out := make([]ServiceProxyEnvSpec, len(serviceProxyEnvSpecs))
	copy(out, serviceProxyEnvSpecs)
	return out
}

func AllowedServiceProxyProviders(ctx context.Context, db *gorm.DB, agent model.Agent) (map[string]bool, error) {
	if db == nil || agent.ID == uuid.Nil || agent.OrgID == nil {
		return nil, nil
	}
	var providers []string
	query := db.WithContext(ctx).
		Table("database_connections").
		Joins("JOIN team_connection_grants ON team_connection_grants.database_connection_id = database_connections.id AND team_connection_grants.org_id = database_connections.org_id").
		Where("team_connection_grants.team_id = ? AND database_connections.org_id = ? AND database_connections.revoked_at IS NULL", agent.TeamID, *agent.OrgID)
	if disabled := agent.ConnectionMCPToolDeny.DisabledConnectionIDs(); len(disabled) > 0 {
		query = query.Where("database_connections.id NOT IN ?", disabled)
	}
	if err := query.
		Distinct("database_connections.provider").
		Pluck("database_connections.provider", &providers).Error; err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(providers))
	for _, provider := range providers {
		allowed[provider] = true
	}
	return allowed, nil
}

func ApplyServiceProxyEnv(env map[string]string, controlPlaneBaseURL string, agentID uuid.UUID, runtimeSecret string, allowed map[string]bool) {
	if env == nil || agentID == uuid.Nil || strings.TrimSpace(controlPlaneBaseURL) == "" || runtimeSecret == "" {
		return
	}
	base := strings.TrimRight(controlPlaneBaseURL, "/")
	for _, spec := range serviceProxyEnvSpecs {
		if allowed != nil && !allowed[spec.Provider] {
			continue
		}
		env[spec.BaseURLEnv] = base + fmt.Sprintf(spec.Path, agentID)
		env[spec.AuthEnv] = runtimeSecret
	}
}

func AgentDriveUploadURL(controlPlaneBaseURL string, agentID, sandboxID uuid.UUID) string {
	if sandboxID == uuid.Nil {
		return ""
	}
	return fmt.Sprintf("%s/internal/agents/%s/sandboxes/%s/drive", strings.TrimRight(controlPlaneBaseURL, "/"), agentID, sandboxID)
}
