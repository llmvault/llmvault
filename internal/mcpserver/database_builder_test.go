package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
