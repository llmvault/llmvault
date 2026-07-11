package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type channelToolFixture struct {
	org     model.Org
	team    model.Team
	agent   model.Agent
	general model.Channel // the team's #general: team-scoped, IsDefault, the channel empty channel_id resolves to
	slack   model.Channel
}

func connectChannelToolTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(testdb.DatabaseURL()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func seedChannelToolFixture(t *testing.T, db *gorm.DB) channelToolFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "channel-tool-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString()}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Hivy", IsDefault: true, Model: "test", Status: "active"}
	// The agent's team #general: team-scoped and IsDefault, mirroring
	// provisionTeamDefaults. An empty channel_id resolves here.
	general := model.Channel{
		ID:             uuid.New(),
		OrgID:          org.ID,
		TeamID:         team.ID,
		Name:           "general-" + uuid.NewString(),
		DefaultAgentID: agent.ID,
		IsDefault:      true,
	}
	slack := model.Channel{
		ID:                   uuid.New(),
		OrgID:                org.ID,
		TeamID:               team.ID,
		Name:                 "alerts-" + uuid.NewString(),
		DefaultAgentID:       agent.ID,
		Origin:               "external",
		ExternalProvider:     "slack",
		ExternalWorkspaceKey: "T0" + uuid.NewString()[:8],
		ExternalResourceKey:  "C0" + uuid.NewString()[:8],
		ExternalResourceName: "alerts",
	}
	for _, row := range []any{&org, &team, &agent, &general, &slack} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed fixture row %T: %v", row, err)
		}
	}
	t.Cleanup(func() { cleanupChannelToolOrg(t, db, org.ID) })
	return channelToolFixture{org: org, team: team, agent: agent, general: general, slack: slack}
}

func cleanupChannelToolOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.AgentTrigger{})
	db.Where("org_id = ?", orgID).Delete(&model.Channel{})
	db.Where("org_id = ?", orgID).Delete(&model.Agent{})
	db.Where("org_id = ?", orgID).Delete(&model.Team{})
	db.Delete(&model.Org{}, "id = ?", orgID)
}

func agentProxyToken(fx channelToolFixture) *model.Token {
	return &model.Token{
		OrgID: fx.org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: fx.agent.ID.String(),
		},
	}
}

func listServerToolNames(t *testing.T, server *mcp.Server) map[string]bool {
	t.Helper()
	ctx := t.Context()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	return names
}

func decodeChannelToolResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("empty tool result")
	}
	if result.IsError {
		t.Fatalf("tool result is error: %v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", result.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("decode tool result %q: %v", text.Text, err)
	}
	return out
}

func channelEntries(t *testing.T, out map[string]any) map[string]map[string]any {
	t.Helper()
	raw, ok := out["channels"].([]any)
	if !ok {
		t.Fatalf("channels missing in %v", out)
	}
	entries := map[string]map[string]any{}
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("channel entry is %T", item)
		}
		id, _ := entry["id"].(string)
		entries[id] = entry
	}
	return entries
}

func TestListChannelsToolGatedToAgentProxyTokens(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	// Register a sentinel tool so the tools capability is advertised even when
	// list_channels is (correctly) not registered.
	server.AddTool(&mcp.Tool{
		Name:        "sentinel",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	addListChannelsTool(server, &model.Token{OrgID: fx.org.ID, Meta: model.JSON{model.TokenMetaType: "api"}}, db)
	if names := listServerToolNames(t, server); names["list_channels"] {
		t.Fatal("list_channels registered for non agent-proxy token")
	}

	proxyServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	addListChannelsTool(proxyServer, agentProxyToken(fx), db)
	if names := listServerToolNames(t, proxyServer); !names["list_channels"] {
		t.Fatal("list_channels not registered for agent-proxy token")
	}

	if err := db.Model(&model.Agent{}).Where("id = ?", fx.agent.ID).Update("is_default", false).Error; err != nil {
		t.Fatalf("make agent non-default: %v", err)
	}
	nonDefaultServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v1"}, nil)
	addListChannelsTool(nonDefaultServer, agentProxyToken(fx), db)
	if names := listServerToolNames(t, nonDefaultServer); names["list_channels"] {
		t.Fatal("list_channels registered for a non-default agent")
	}
}

func TestListChannelsReturnsOrgChannelsWithSlackLinkage(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)
	other := seedChannelToolFixture(t, db)

	result, err := handleListChannels(t.Context(), db, &fx.agent, "")
	if err != nil {
		t.Fatalf("handleListChannels: %v", err)
	}
	entries := channelEntries(t, decodeChannelToolResult(t, result))

	general, ok := entries[fx.general.ID.String()]
	if !ok {
		t.Fatalf("general channel missing from %v", entries)
	}
	if general["name"] != fx.general.Name || general["kind"] != "standard" || general["visibility"] != "public" {
		t.Fatalf("general entry = %v", general)
	}
	if general["is_default"] != true {
		t.Fatalf("general flags = %v", general)
	}
	if _, ok := general["is_system"]; ok {
		t.Fatalf("is_system should no longer be emitted: %v", general)
	}
	if _, ok := general["external_provider"]; ok {
		t.Fatalf("native channel should omit external fields: %v", general)
	}

	slack, ok := entries[fx.slack.ID.String()]
	if !ok {
		t.Fatalf("slack channel missing from %v", entries)
	}
	if slack["external_provider"] != "slack" ||
		slack["external_resource_key"] != fx.slack.ExternalResourceKey ||
		slack["external_resource_name"] != "alerts" {
		t.Fatalf("slack entry = %v", slack)
	}

	if _, ok := entries[other.general.ID.String()]; ok {
		t.Fatal("cross-org channel visible in list_channels")
	}
	if _, ok := entries[other.slack.ID.String()]; ok {
		t.Fatal("cross-org slack channel visible in list_channels")
	}
}

// TestListChannelsExcludesForeignTeamChannels verifies the team-primary rule
// (channelagents.ActsInChannel) that replaced the cut agent_channels allowlist:
// list_channels surfaces only channels the calling agent can act in — its own
// team's channels and shared/team-less channels — and excludes channels owned by
// a team the agent does not belong to.
func TestListChannelsExcludesForeignTeamChannels(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)
	foreignTeam := model.Team{ID: uuid.New(), OrgID: fx.org.ID, Name: "foreign-" + uuid.NewString()}
	if err := db.Create(&foreignTeam).Error; err != nil {
		t.Fatalf("create foreign team: %v", err)
	}
	foreignChannel := model.Channel{ID: uuid.New(), OrgID: fx.org.ID, TeamID: foreignTeam.ID, Name: "foreign-" + uuid.NewString(), DefaultAgentID: fx.agent.ID}
	if err := db.Create(&foreignChannel).Error; err != nil {
		t.Fatalf("create foreign channel: %v", err)
	}

	result, err := handleListChannels(t.Context(), db, &fx.agent, "")
	if err != nil {
		t.Fatalf("handleListChannels: %v", err)
	}
	entries := channelEntries(t, decodeChannelToolResult(t, result))
	if _, ok := entries[foreignChannel.ID.String()]; ok {
		t.Fatalf("foreign-team channel should be excluded: %v", entries)
	}
	if _, ok := entries[fx.general.ID.String()]; !ok {
		t.Fatalf("agent's team channel missing from %v", entries)
	}
	if _, ok := entries[fx.slack.ID.String()]; !ok {
		t.Fatalf("shared slack channel missing from %v", entries)
	}
}
