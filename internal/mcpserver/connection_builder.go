package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionaccess"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

// BuildConnectionServer creates a server containing only the executable
// catalog actions for one exact Nango connection.
func BuildConnectionServer(
	ctx context.Context,
	db *gorm.DB,
	nangoClient *nango.Client,
	cat *catalog.Catalog,
	token *model.Token,
	connectionID uuid.UUID,
) (*mcp.Server, error) {
	if db == nil || nangoClient == nil || cat == nil {
		return nil, fmt.Errorf("connection MCP dependencies are unavailable")
	}
	agentID, err := proxyAgentID(token)
	if err != nil {
		return nil, err
	}
	resolved, err := connectionaccess.ResolveAgentConnection(ctx, db, token.OrgID, agentID, connectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve connection MCP scope: %w", err)
	}
	providerActions, ok := cat.GetProvider(resolved.Connection.Integration.Provider)
	if !ok || !providerActions.ShouldPushToMCP() {
		return nil, fmt.Errorf("connection provider has no MCP catalog")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: resolved.Connection.Slug, Version: "v1.0.0"}, nil)
	keys := make([]string, 0, len(providerActions.Actions))
	for key, action := range providerActions.Actions {
		if action.Execution != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		action := providerActions.Actions[key]
		var inputSchema map[string]any
		if err := json.Unmarshal(action.Parameters, &inputSchema); err != nil {
			return nil, fmt.Errorf("decode action %s input schema: %w", key, err)
		}
		server.AddTool(&mcp.Tool{
			Name:        key,
			Title:       action.DisplayName,
			Description: action.Description,
			InputSchema: inputSchema,
		}, func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params map[string]any
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &params); err != nil {
					return connectionToolError("invalid tool arguments"), nil
				}
			}
			if params == nil {
				params = map[string]any{}
			}
			// Resolve again for every call so revocation or grant removal takes
			// effect even if this server instance remains in the JTI cache.
			current, err := connectionaccess.ResolveAgentConnection(callCtx, db, token.OrgID, agentID, connectionID)
			if err != nil {
				return connectionToolError("connection is no longer available"), nil
			}
			result, err := ExecuteAction(
				callCtx,
				nangoClient,
				current.Connection.Integration.Provider,
				current.ProviderConfigKey,
				current.Connection.NangoConnectionID,
				&action,
				params,
				resourceAllowList(current.Resources),
				providerActions.Schemas,
			)
			if err != nil {
				logging.FromContext(callCtx).ErrorContext(callCtx, "connection MCP tool failed", "error", err, "connection_id", connectionID, "tool", key)
				return connectionToolError("provider request failed"), nil
			}
			return connectionToolJSON(result)
		})
	}
	return server, nil
}

func proxyAgentID(token *model.Token) (uuid.UUID, error) {
	if token == nil || token.Meta == nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is required")
	}
	raw, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(raw)
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token has no agent id")
	}
	return agentID, nil
}

func resourceAllowList(resources model.JSON) map[string][]string {
	out := make(map[string][]string, len(resources))
	for key, raw := range resources {
		switch values := raw.(type) {
		case []string:
			out[key] = append([]string(nil), values...)
		case []any:
			for _, value := range values {
				if text, ok := value.(string); ok {
					out[key] = append(out[key], text)
				}
			}
		}
	}
	return out
}

func connectionToolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + message}}}
}

func connectionToolJSON(value any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode tool result: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
}
