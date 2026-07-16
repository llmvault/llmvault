package agentruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

type connectionMCPRequirement struct {
	PluginID uuid.UUID
	Provider string
	Kind     string
}

func resolveConnectionMCPServers(ctx context.Context, deps CompileDeps, agent *model.Agent, proxyToken *ProxyTokenResult) ([]any, error) {
	if deps.DB == nil || deps.Cfg == nil || strings.TrimSpace(deps.Cfg.MCPBaseURL) == "" || agent == nil || agent.OrgID == nil {
		return []any{}, nil
	}
	jti := ""
	if proxyToken != nil {
		jti = proxyToken.JTI
	}
	if jti == "" {
		var token model.Token
		if err := deps.DB.WithContext(ctx).
			Where("org_id = ? AND expires_at > ? AND revoked_at IS NULL", *agent.OrgID, time.Now()).
			Where("meta->>? = ? AND meta->>? = ?", model.TokenMetaAgentID, agent.ID.String(), model.TokenMetaType, model.TokenTypeAgentProxy).
			Order("created_at DESC").First(&token).Error; err != nil {
			return []any{}, nil
		}
		jti = token.JTI
	}
	pluginIDs, err := pluginresolve.EffectivePluginIDs(ctx, deps.DB, *agent)
	if err != nil {
		return nil, fmt.Errorf("resolve connection MCP plugins: %w", err)
	}
	if len(pluginIDs) == 0 {
		return []any{}, nil
	}
	var requirements []connectionMCPRequirement
	if err := deps.DB.WithContext(ctx).Model(&model.PluginIntegration{}).
		Select("plugin_id, provider, kind").Where("plugin_id IN ?", pluginIDs).
		Scan(&requirements).Error; err != nil {
		return nil, fmt.Errorf("load connection MCP requirements: %w", err)
	}
	integrationProviders := make([]string, 0)
	databaseProviders := make([]string, 0)
	for _, requirement := range requirements {
		switch requirement.Kind {
		case model.PluginIntegrationKindDatabase:
			databaseProviders = append(databaseProviders, requirement.Provider)
		default:
			integrationProviders = append(integrationProviders, requirement.Provider)
		}
	}
	baseURL := strings.TrimRight(deps.Cfg.MCPBaseURL, "/")
	toolDenyByProvider := pluginMCPToolDenyByProvider(agent.PluginMCPToolDeny, requirements)
	servers := make([]any, 0)
	if len(integrationProviders) > 0 {
		var connections []model.Connection
		if err := deps.DB.WithContext(ctx).Preload("Integration").
			Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
			Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider IN ?", *agent.OrgID, integrationProviders).
			Order("connections.created_at ASC, connections.id ASC").Find(&connections).Error; err != nil {
			return nil, fmt.Errorf("load connection MCP connections: %w", err)
		}
		cat := catalog.Global()
		for _, connection := range connections {
			provider, ok := cat.GetProvider(connection.Integration.Provider)
			if !ok || !provider.ShouldPushToMCP() || executableActionCount(provider) == 0 {
				continue
			}
			servers = append(servers, connectionMCPServer(
				"connection",
				connection.Slug,
				connection.ID,
				baseURL,
				jti,
				toolDenyByProvider[pluginMCPProviderKey(model.PluginIntegrationKindIntegration, connection.Integration.Provider)],
			))
		}
	}
	if len(databaseProviders) > 0 {
		var connections []model.DatabaseConnection
		if err := deps.DB.WithContext(ctx).
			Where("org_id = ? AND revoked_at IS NULL AND provider IN ?", *agent.OrgID, databaseProviders).
			Order("created_at ASC, id ASC").Find(&connections).Error; err != nil {
			return nil, fmt.Errorf("load database MCP connections: %w", err)
		}
		for _, connection := range connections {
			servers = append(servers, connectionMCPServer(
				"database",
				connection.Slug,
				connection.ID,
				baseURL,
				jti,
				toolDenyByProvider[pluginMCPProviderKey(model.PluginIntegrationKindDatabase, connection.Provider)],
			))
		}
	}
	sort.Slice(servers, func(i, j int) bool {
		left, _ := servers[i].(map[string]any)["name"].(string)
		right, _ := servers[j].(map[string]any)["name"].(string)
		return left < right
	})
	return servers, nil
}

func executableActionCount(provider *catalog.ProviderActions) int {
	count := 0
	for _, action := range provider.Actions {
		if action.Execution != nil {
			count++
		}
	}
	return count
}

func connectionMCPServer(kind, slug string, connectionID uuid.UUID, baseURL, jti string, deniedTools []string) any {
	name := kind + "-" + strings.Trim(strings.TrimSpace(slug), "-")
	return map[string]any{
		"name":      name,
		"transport": "streamable_http",
		"url":       fmt.Sprintf("%s/%s/%s/%s", baseURL, jti, kind, connectionID),
		"headers": map[string]string{
			"Authorization": agentMCPAuthorizationHeader(),
		},
		// Generated plugin MCPs are deny-based and therefore default to the full
		// catalog. The explicit per-server filter also tells the runtime not to
		// apply the legacy top-level Hivy MCP allow-list to this server.
		"tool_filter": map[string]any{
			"deny": append([]string(nil), deniedTools...),
		},
	}
}

func pluginMCPToolDenyByProvider(config model.PluginMCPToolDeny, requirements []connectionMCPRequirement) map[string][]string {
	sets := make(map[string]map[string]bool)
	for _, requirement := range requirements {
		denied := config[requirement.PluginID.String()]
		if len(denied) == 0 {
			continue
		}
		key := pluginMCPProviderKey(requirement.Kind, requirement.Provider)
		if sets[key] == nil {
			sets[key] = make(map[string]bool, len(denied))
		}
		for _, tool := range denied {
			if tool = strings.TrimSpace(tool); tool != "" {
				sets[key][tool] = true
			}
		}
	}
	out := make(map[string][]string, len(sets))
	for key, set := range sets {
		for tool := range set {
			out[key] = append(out[key], tool)
		}
		sort.Strings(out[key])
	}
	return out
}

func pluginMCPProviderKey(kind, provider string) string {
	return kind + ":" + provider
}
