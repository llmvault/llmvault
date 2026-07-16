package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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
	server.AddTool(databaseQueryTool(connection), func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			return connectionToolError(databaseQueryError(err)), nil
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
	if agent.ConnectionMCPToolDeny.DisablesConnection(connectionID) {
		return model.DatabaseConnection{}, gorm.ErrRecordNotFound
	}
	var connection model.DatabaseConnection
	err := db.WithContext(ctx).
		Joins("JOIN team_connection_grants tcg ON tcg.database_connection_id = database_connections.id AND tcg.org_id = database_connections.org_id").
		Where("tcg.team_id = ?", agent.TeamID).
		Where("database_connections.id = ? AND database_connections.org_id = ? AND database_connections.revoked_at IS NULL", connectionID, orgID).
		First(&connection).Error
	return connection, err
}

func databaseQueryTool(connection model.DatabaseConnection) *mcp.Tool {
	tool := &mcp.Tool{Name: "run_query"}
	connectionSlug := connection.Slug
	if connectionSlug == "" {
		connectionSlug = connection.Provider
	}
	switch connection.Provider {
	case dbi.ProviderMongoDB:
		tool.Description = fmt.Sprintf(
			"Run a read-only command against the **MongoDB** connection `%s`."+
				"\n\n**How to use**\n\n"+
				"- Set `command` to one MongoDB command document using `find`, `aggregate`, `count`, or `distinct`.\n"+
				"- Example: `{\"find\":\"orders\",\"filter\":{\"status\":\"open\"},\"limit\":25}`.\n"+
				"- The connection policy limits accessible collections, returned fields, and result count.\n"+
				"- Write, schema, privilege, and unsupported commands are rejected.",
			connectionSlug,
		)
		tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "object", "description": "One MongoDB `find`, `aggregate`, `count`, or `distinct` command document."}}, "required": []string{"command"}}
	case dbi.ProviderRedis:
		tool.Description = fmt.Sprintf(
			"Run a read-only command against the **Redis** connection `%s`."+
				"\n\n**How to use**\n\n"+
				"- Put the command name in `command` and its positional arguments in `args`.\n"+
				"- Example: `{\"command\":\"GET\",\"args\":[\"user:123\"]}`.\n"+
				"- Supported commands include `GET`, `MGET`, `HGET`, `LRANGE`, `SCAN`, `TTL`, and other read-only Redis commands.\n"+
				"- The connection policy limits keys, scan patterns, ranges, and result count; write or administrative commands are rejected.",
			connectionSlug,
		)
		tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string", "description": "A read-only Redis command name, such as `GET`, `HGET`, or `SCAN`."}, "args": map[string]any{"type": "array", "description": "Positional command arguments, including keys and optional range or scan values.", "items": map[string]any{}}}, "required": []string{"command"}}
	case dbi.ProviderMySQL:
		tool.Description = fmt.Sprintf(
			"Run a read-only SQL query against the **MySQL** connection `%s`."+
				"\n\n**How to use**\n\n"+
				"- Set `query` to one `SELECT`, `WITH`, or `EXPLAIN` statement. `SHOW` and `DESCRIBE` also work when the connection has no schema or table allowlist.\n"+
				"- Select only the columns you need, and add a small `LIMIT` while exploring data.\n"+
				"- Results contain `columns`, `rows`, `row_count`, and `truncated`.\n"+
				"- The connection policy limits accessible schemas, tables, fields, and result count; writes, DDL, privilege changes, and multiple statements are rejected.",
			connectionSlug,
		)
		tool.InputSchema = databaseSQLInputSchema()
	default:
		tool.Description = fmt.Sprintf(
			"Run a read-only SQL query against the **PostgreSQL** connection `%s`."+
				"\n\n**How to use**\n\n"+
				"- Set `query` to one `SELECT`, `WITH`, or `EXPLAIN` statement.\n"+
				"- Always qualify every table as `<schema>.<table>` (for example, `public.users`). Never use a bare table name such as `users`.\n"+
				"- Select only the columns you need, and add a small `LIMIT` while exploring data.\n"+
				"- Results contain `columns`, `rows`, `row_count`, and `truncated`.\n"+
				"- The connection policy limits accessible schemas, tables, fields, and result count; writes, DDL, privilege changes, and multiple statements are rejected.",
			connectionSlug,
		)
		tool.InputSchema = databaseSQLInputSchema()
	}
	return tool
}

func databaseQueryError(err error) string {
	if errors.Is(err, dbi.ErrSchemaQualificationRequired) {
		return err.Error()
	}
	return "database query failed"
}

func databaseSQLInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "One read-only SQL statement. Use an explicit column list and a small `LIMIT` for exploratory queries.",
			},
		},
		"required": []string{"query"},
	}
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
