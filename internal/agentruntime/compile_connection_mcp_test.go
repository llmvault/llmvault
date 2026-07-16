package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestResolveConnectionMCPServersEmitsOneServerPerMatchingConnection(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createCompileTokenAgent(t, db)
	orgID := *agent.OrgID
	user := model.User{Email: fmt.Sprintf("connection-mcp-%s@example.com", uuid.NewString())}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plugin := model.Plugin{Slug: "connection-mcp-" + uuid.NewString(), Name: "Connection MCP", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	for _, requirement := range []model.PluginIntegration{
		{PluginID: plugin.ID, Provider: "slack", Kind: model.PluginIntegrationKindIntegration, Required: true},
		{PluginID: plugin.ID, Provider: "postgres", Kind: model.PluginIntegrationKindDatabase, Required: true},
	} {
		if err := db.Create(&requirement).Error; err != nil {
			t.Fatalf("create plugin requirement: %v", err)
		}
	}
	if err := db.Create(&model.OrgPluginInstall{OrgID: orgID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	if err := db.Create(&model.TeamPlugin{OrgID: orgID, TeamID: agent.TeamID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("grant plugin: %v", err)
	}
	agent.PluginMCPToolDeny = model.PluginMCPToolDeny{
		plugin.ID.String(): {"chat_delete", "run_query"},
	}
	integration := model.Integration{UniqueKey: "connection-mcp-" + uuid.NewString(), Provider: "slack", DisplayName: "Slack"}
	if err := db.Create(&integration).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	for index, slug := range []string{"slack", "sales"} {
		connection := model.Connection{OrgID: orgID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: fmt.Sprintf("nango-%d", index), Name: slug, Slug: slug}
		if err := db.Create(&connection).Error; err != nil {
			t.Fatalf("create connection: %v", err)
		}
	}
	for _, slug := range []string{"postgres", "reporting"} {
		connection := model.DatabaseConnection{OrgID: orgID, Provider: "postgres", DisplayName: "Postgres", Name: slug, Slug: slug, EncryptedDSN: []byte("encrypted"), WrappedDEK: []byte("wrapped"), SchemaSnapshot: model.RawJSON(`{}`), AccessPolicy: model.JSON{}}
		if err := db.Create(&connection).Error; err != nil {
			t.Fatalf("create database connection: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.DatabaseConnection{})
		db.Where("org_id = ?", orgID).Delete(&model.Connection{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).Delete(&model.OrgPluginInstall{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{})
		db.Delete(&integration)
		db.Delete(&plugin)
		db.Delete(&user)
	})

	servers, err := resolveConnectionMCPServers(context.Background(), CompileDeps{
		DB:  db,
		Cfg: &config.Config{MCPBaseURL: "https://mcp.example.test"},
	}, &agent, &ProxyTokenResult{JTI: "test-jti", Token: "ptok_test"})
	if err != nil {
		t.Fatalf("resolve connection MCP servers: %v", err)
	}
	if len(servers) != 4 {
		t.Fatalf("server count = %d, want 4: %#v", len(servers), servers)
	}
	wantNames := []string{"connection-sales", "connection-slack", "database-postgres", "database-reporting"}
	wantPrefixes := map[string]string{
		"database-postgres":  "postgres_primary",
		"database-reporting": "postgres_reporting",
	}
	for index, raw := range servers {
		server := raw.(map[string]any)
		if server["name"] != wantNames[index] {
			t.Fatalf("server %d name = %v, want %s", index, server["name"], wantNames[index])
		}
		if wantPrefix := wantPrefixes[wantNames[index]]; wantPrefix != "" {
			if server["tool_name_prefix"] != wantPrefix {
				t.Fatalf("server %s tool prefix = %v, want %s", wantNames[index], server["tool_name_prefix"], wantPrefix)
			}
		} else if _, ok := server["tool_name_prefix"]; ok {
			t.Fatalf("integration server %s unexpectedly has a tool prefix", wantNames[index])
		}
		if url := server["url"].(string); !strings.Contains(url, "/test-jti/") {
			t.Fatalf("server URL = %q", url)
		}
		filter, ok := server["tool_filter"].(map[string]any)
		if !ok {
			t.Fatalf("server %s tool filter = %#v", wantNames[index], server["tool_filter"])
		}
		deny, ok := filter["deny"].([]string)
		if !ok || len(deny) != 2 || deny[0] != "chat_delete" || deny[1] != "run_query" {
			t.Fatalf("server %s deny = %#v, want chat_delete+run_query", wantNames[index], filter["deny"])
		}
	}
}

func TestDatabaseMCPToolPrefixUsesProviderAndConnectionLabel(t *testing.T) {
	tests := map[string]struct {
		provider string
		slug     string
		want     string
	}{
		"default postgres": {provider: "postgres", slug: "postgres", want: "postgres_primary"},
		"named postgres":   {provider: "postgres", slug: "reporting", want: "postgres_reporting"},
		"named mysql":      {provider: "mysql", slug: "analytics", want: "mysql_analytics"},
		"named redis":      {provider: "redis", slug: "cache", want: "redis_cache"},
		"named mongodb":    {provider: "mongodb", slug: "archive", want: "mongodb_archive"},
		"hyphenated mongo": {provider: "mongodb", slug: "cold-archive", want: "mongodb_cold_archive"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := databaseMCPToolPrefix(test.provider, test.slug); got != test.want {
				t.Fatalf("database MCP tool prefix = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPluginMCPToolDenyByProviderUnionsPluginsForSharedProvider(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	got := pluginMCPToolDenyByProvider(model.PluginMCPToolDeny{
		first.String():  {"chat_delete", "chat_update"},
		second.String(): {"chat_delete", "reactions_remove"},
	}, []connectionMCPRequirement{
		{PluginID: first, Provider: "slack", Kind: model.PluginIntegrationKindIntegration},
		{PluginID: second, Provider: "slack", Kind: model.PluginIntegrationKindIntegration},
	})
	want := []string{"chat_delete", "chat_update", "reactions_remove"}
	if joined := strings.Join(got[pluginMCPProviderKey(model.PluginIntegrationKindIntegration, "slack")], ","); joined != strings.Join(want, ",") {
		t.Fatalf("shared-provider deny = %q, want %q", joined, strings.Join(want, ","))
	}
}

func TestPluginMCPToolDenyPreservesLegacyDatabaseQueryOptOut(t *testing.T) {
	pluginID := uuid.New()
	got := pluginMCPToolDenyByProvider(model.PluginMCPToolDeny{
		pluginID.String(): {"query"},
	}, []connectionMCPRequirement{
		{PluginID: pluginID, Provider: "postgres", Kind: model.PluginIntegrationKindDatabase},
	})
	denied := got[pluginMCPProviderKey(model.PluginIntegrationKindDatabase, "postgres")]
	if len(denied) != 1 || denied[0] != "run_query" {
		t.Fatalf("legacy database deny = %v, want run_query", denied)
	}
}
