package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func buildAgentMCPServer(ctx context.Context, deps CompileDeps, agent *model.Agent) any {
	return buildHivyMCPServer(ctx, deps, agent)
}

func buildAgentMCPServerWithToken(deps CompileDeps, token *ProxyTokenResult) any {
	return buildHivyMCPServerWithToken(deps, token)
}

func buildHivyMCPServerWithToken(deps CompileDeps, token *ProxyTokenResult) any {
	if deps.Cfg == nil || deps.Cfg.MCPBaseURL == "" || token == nil || token.JTI == "" {
		return nil
	}
	return hivyMCPServer(deps.Cfg.MCPBaseURL, token.JTI)
}

func buildHivyMCPServer(ctx context.Context, deps CompileDeps, agent *model.Agent) any {
	if deps.DB == nil || deps.Cfg == nil || deps.Cfg.MCPBaseURL == "" || agent.OrgID == nil {
		return nil
	}
	var tok model.Token
	q := deps.DB.WithContext(ctx).
		Where("org_id = ? AND expires_at > ? AND revoked_at IS NULL", *agent.OrgID, time.Now()).
		Where("meta->>? = ? AND meta->>? = ?",
			model.TokenMetaAgentID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeAgentProxy)
	if err := q.
		Order("created_at DESC").
		First(&tok).Error; err != nil {
		return nil
	}
	return hivyMCPServer(deps.Cfg.MCPBaseURL, tok.JTI)
}

func hivyMCPServer(baseURL, jti string) any {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), jti)
	return map[string]any{
		"name":      "hivy",
		"transport": "streamable_http",
		"url":       url,
		"headers": map[string]string{
			"Authorization": agentMCPAuthorizationHeader(),
		},
	}
}

func agentMCPAuthorizationHeader() string {
	return "Bearer ${" + ProxyAPIKeyEnv + "}"
}

func upsertHivyMCPServer(servers []any, server any) []any {
	out := make([]any, 0, len(servers)+1)
	for _, existing := range servers {
		if m, ok := existing.(map[string]any); ok {
			if name, _ := m["name"].(string); name == "hivy" {
				continue
			}
		}
		out = append(out, existing)
	}
	return append(out, server)
}
