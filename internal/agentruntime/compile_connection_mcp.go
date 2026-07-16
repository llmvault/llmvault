package agentruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

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
	baseURL := strings.TrimRight(deps.Cfg.MCPBaseURL, "/")
	servers := make([]any, 0)
	connections, err := connectionaccess.IntegrationConnections(ctx, deps.DB, *agent)
	if err != nil {
		return nil, err
	}
	cat := catalog.Global()
	for _, connection := range connections {
		provider, ok := cat.GetProvider(connection.Integration.Provider)
		if !ok || !provider.ShouldPushToMCP() || executableActionCount(provider) == 0 {
			continue
		}
		servers = append(servers, connectionMCPServer(
			"connection", connection.Slug, "", connection.ID, baseURL, jti,
			connectionToolDeny(agent.ConnectionMCPToolDeny, connection.ID),
		))
	}
	databaseConnections, err := connectionaccess.DatabaseConnections(ctx, deps.DB, *agent)
	if err != nil {
		return nil, err
	}
	for _, connection := range databaseConnections {
		servers = append(servers, connectionMCPServer(
			"database", connection.Slug, databaseMCPToolPrefix(connection.Provider, connection.Slug),
			connection.ID, baseURL, jti,
			connectionToolDeny(agent.ConnectionMCPToolDeny, connection.ID),
		))
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

func connectionMCPServer(kind, slug, toolNamePrefix string, connectionID uuid.UUID, baseURL, jti string, deniedTools []string) any {
	name := kind + "-" + strings.Trim(strings.TrimSpace(slug), "-")
	server := map[string]any{
		"name":      name,
		"transport": "streamable_http",
		"url":       fmt.Sprintf("%s/%s/%s/%s", baseURL, jti, kind, connectionID),
		"headers": map[string]string{
			"Authorization": agentMCPAuthorizationHeader(),
		},
		"tool_filter": map[string]any{"deny": deniedTools},
	}
	if toolNamePrefix != "" {
		server["tool_name_prefix"] = toolNamePrefix
	}
	return server
}

func databaseMCPToolPrefix(provider, slug string) string {
	provider = strings.Trim(strings.ToLower(strings.TrimSpace(provider)), "-_")
	label := strings.Trim(strings.ToLower(strings.TrimSpace(slug)), "-_")
	if label == "" || label == provider {
		label = "primary"
	}
	provider = strings.ReplaceAll(provider, "-", "_")
	label = strings.ReplaceAll(label, "-", "_")
	return provider + "_" + label
}

func connectionToolDeny(config model.ConnectionMCPToolDeny, connectionID uuid.UUID) []string {
	values := config[connectionID.String()]
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
