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
	for index, raw := range servers {
		server := raw.(map[string]any)
		if server["name"] != wantNames[index] {
			t.Fatalf("server %d name = %v, want %s", index, server["name"], wantNames[index])
		}
		if url := server["url"].(string); !strings.Contains(url, "/test-jti/") {
			t.Fatalf("server URL = %q", url)
		}
	}
}
