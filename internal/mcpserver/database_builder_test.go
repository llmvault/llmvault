package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/model"
)

func TestDatabaseServerListsRunQueryTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "postgres-primary", Version: "v1"}, nil)
	server.AddTool(databaseQueryTool(model.DatabaseConnection{Provider: "postgres", Slug: "postgres"}), func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})

	names := listServerToolNames(t, server)
	if !names["run_query"] || names["query"] {
		t.Fatalf("database tools = %v, want only renamed run_query", names)
	}
}

func TestResolveAgentDatabaseConnectionHonorsAgentOptOut(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)
	connection := model.DatabaseConnection{
		ID:           uuid.New(),
		OrgID:        fx.org.ID,
		Provider:     "postgres",
		DisplayName:  "Reporting",
		Name:         "reporting",
		Slug:         "reporting",
		EncryptedDSN: []byte("encrypted"),
		WrappedDEK:   []byte("wrapped"),
		AccessPolicy: model.JSON{},
	}
	grant := model.TeamConnectionGrant{
		ID:                   uuid.New(),
		OrgID:                fx.org.ID,
		TeamID:               fx.team.ID,
		DatabaseConnectionID: &connection.ID,
	}
	for _, row := range []any{&connection, &grant} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed database MCP row %T: %v", row, err)
		}
	}

	if _, err := resolveAgentDatabaseConnection(t.Context(), db, fx.org.ID, fx.agent.ID, connection.ID); err != nil {
		t.Fatalf("resolve inherited database connection: %v", err)
	}
	deny := model.ConnectionMCPToolDeny{
		connection.ID.String(): {model.ConnectionMCPToolDenyAll},
	}
	if err := db.Model(&model.Agent{}).
		Where("id = ? AND org_id = ?", fx.agent.ID, fx.org.ID).
		Update("connection_mcp_tool_deny", deny).Error; err != nil {
		t.Fatalf("disable inherited database connection for agent: %v", err)
	}
	if _, err := resolveAgentDatabaseConnection(t.Context(), db, fx.org.ID, fx.agent.ID, connection.ID); err == nil {
		t.Fatal("expected disabled inherited database connection to be unavailable")
	}
}

func TestDatabaseToolDescriptionsExplainHowToUseEachProvider(t *testing.T) {
	tests := map[string]struct {
		connection model.DatabaseConnection
		fragments  []string
	}{
		"postgres": {
			connection: model.DatabaseConnection{Provider: "postgres", Slug: "reporting"},
			fragments: []string{
				"**PostgreSQL** connection `reporting`",
				"**How to use**",
				"`SELECT`, `WITH`, or `EXPLAIN`",
				"Always qualify every table as `<schema>.<table>`",
				"Never use a bare table name",
				"`columns`, `rows`, `row_count`, and `truncated`",
				"multiple statements are rejected",
			},
		},
		"mysql": {
			connection: model.DatabaseConnection{Provider: "mysql", Slug: "analytics"},
			fragments: []string{
				"**MySQL** connection `analytics`",
				"**How to use**",
				"`SHOW` and `DESCRIBE`",
				"schema or table allowlist",
				"multiple statements are rejected",
			},
		},
		"mongodb": {
			connection: model.DatabaseConnection{Provider: "mongodb", Slug: "archive"},
			fragments: []string{
				"**MongoDB** connection `archive`",
				"**How to use**",
				"`find`, `aggregate`, `count`, or `distinct`",
				`{"find":"orders","filter":{"status":"open"},"limit":25}`,
				"unsupported commands are rejected",
			},
		},
		"redis": {
			connection: model.DatabaseConnection{Provider: "redis", Slug: "cache"},
			fragments: []string{
				"**Redis** connection `cache`",
				"**How to use**",
				"command name in `command`",
				`{"command":"GET","args":["user:123"]}`,
				"write or administrative commands are rejected",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: name, Version: "v1"}, nil)
			server.AddTool(databaseQueryTool(test.connection), func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{}, nil
			})

			tool := listServerTools(t, server)["run_query"]
			if tool == nil {
				t.Fatal("run_query tool not listed")
			}
			for _, fragment := range test.fragments {
				if !strings.Contains(tool.Description, fragment) {
					t.Errorf("description missing %q:\n%s", fragment, tool.Description)
				}
			}
		})
	}
}

func TestDatabaseQueryErrorExposesSchemaQualificationGuidance(t *testing.T) {
	err := fmt.Errorf("%w: always use <schema>.<table>", dbi.ErrSchemaQualificationRequired)
	if got := databaseQueryError(err); got != err.Error() {
		t.Fatalf("databaseQueryError() = %q, want %q", got, err.Error())
	}
	if got := databaseQueryError(fmt.Errorf("driver failed")); got != "database query failed" {
		t.Fatalf("databaseQueryError() leaked internal error: %q", got)
	}
}
