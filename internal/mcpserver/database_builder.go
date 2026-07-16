package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

// BuildDatabaseServer creates the single read-only query tool for one exact
// database connection.
func BuildDatabaseServer(ctx context.Context, db *gorm.DB, kms *crypto.KeyWrapper, token *model.Token, connectionID uuid.UUID) (*mcp.Server, error) {
	if db == nil || kms == nil {
		return nil, fmt.Errorf("database MCP dependencies are unavailable")
	}
	agentID, err := proxyAgentID(token)
	if err != nil {
		return nil, err
	}
	connection, err := resolveAgentDatabaseConnection(ctx, db, token.OrgID, agentID, connectionID)
	if err != nil {
		return nil, fmt.Errorf("resolve database MCP scope: %w", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: connection.Slug, Version: "v1.0.0"}, nil)
	server.AddTool(databaseQueryTool(connection.Provider), func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		current, err := resolveAgentDatabaseConnection(callCtx, db, token.OrgID, agentID, connectionID)
		if err != nil {
			return connectionToolError("database connection is no longer available"), nil
		}
		body, err := databaseToolBody(current.Provider, req)
		if err != nil {
			return connectionToolError(err.Error()), nil
		}
		dsn, err := dbi.DecryptSecret(callCtx, kms, current.EncryptedDSN, current.WrappedDEK)
		if err != nil {
			logging.FromContext(callCtx).ErrorContext(callCtx, "decrypt database MCP connection", "error", err, "connection_id", connectionID)
			return connectionToolError("database connection could not be opened"), nil
		}
		result, err := dbi.Execute(callCtx, current.Provider, dsn, body, dbi.PolicyFromJSON(current.AccessPolicy))
		if err != nil {
			logging.FromContext(callCtx).ErrorContext(callCtx, "database MCP query failed", "error", err, "connection_id", connectionID)
			return connectionToolError("database query failed"), nil
		}
		return connectionToolJSON(result)
	})
	return server, nil
}

func resolveAgentDatabaseConnection(ctx context.Context, db *gorm.DB, orgID, agentID, connectionID uuid.UUID) (model.DatabaseConnection, error) {
	var agent model.Agent
	if err := db.WithContext(ctx).Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").First(&agent).Error; err != nil {
		return model.DatabaseConnection{}, err
	}
	pluginIDs, err := pluginresolve.EffectivePluginIDs(ctx, db, agent)
	if err != nil || len(pluginIDs) == 0 {
		if err != nil {
			return model.DatabaseConnection{}, err
		}
		return model.DatabaseConnection{}, gorm.ErrRecordNotFound
	}
	var connection model.DatabaseConnection
	err = db.WithContext(ctx).
		Joins("JOIN plugin_integrations ON plugin_integrations.provider = database_connections.provider AND plugin_integrations.kind = ?", model.PluginIntegrationKindDatabase).
		Where("plugin_integrations.plugin_id IN ?", pluginIDs).
		Where("database_connections.id = ? AND database_connections.org_id = ? AND database_connections.revoked_at IS NULL", connectionID, orgID).
		First(&connection).Error
	return connection, err
}

func databaseQueryTool(provider string) *mcp.Tool {
	tool := &mcp.Tool{Name: "query"}
	switch provider {
	case dbi.ProviderMongoDB:
		tool.Description = "Run one read-only MongoDB command against this connection."
		tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "object", "description": "MongoDB find, aggregate, count, or distinct command."}}, "required": []string{"command"}}
	case dbi.ProviderRedis:
		tool.Description = "Run one read-only Redis command against this connection."
		tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}, "args": map[string]any{"type": "array", "items": map[string]any{}}}, "required": []string{"command"}}
	default:
		tool.Description = "Run one read-only SQL query against this connection."
		tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string", "description": "One read-only SQL statement."}}, "required": []string{"query"}}
	}
	return tool
}

func databaseToolBody(provider string, req *mcp.CallToolRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("tool arguments are required")
	}
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments")
	}
	if provider == dbi.ProviderMongoDB {
		command, ok := args["command"].(map[string]any)
		if !ok || len(command) == 0 {
			return nil, fmt.Errorf("command is required")
		}
		return json.Marshal(command)
	}
	if provider == dbi.ProviderRedis {
		command, _ := args["command"].(string)
		if command == "" {
			return nil, fmt.Errorf("command is required")
		}
		return json.Marshal(map[string]any{"command": args["command"], "args": args["args"]})
	}
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	return []byte(query), nil
}
