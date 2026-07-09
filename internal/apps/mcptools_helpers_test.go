package apps

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// appToolNames is the full expected tool group, in registration order.
var appToolNames = []string{
	toolAppCreate, toolAppPublish, toolAppStatus, toolAppLogs, toolAppRollback,
}

// ensureAppsPlugin returns the active plugin row with slug "apps", creating it
// when the test DB has not synced global plugins. The row is intentionally NOT
// cleaned up: the slug is unique and shared across tests.
func ensureAppsPlugin(t *testing.T, db *gorm.DB) model.Plugin {
	t.Helper()
	var plugin model.Plugin
	err := db.Where("slug = ? AND org_id IS NULL", AppsPluginSlug).First(&plugin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		plugin = model.Plugin{Slug: AppsPluginSlug, Name: "Apps", Status: model.PluginStatusActive, Manifest: model.RawJSON(`{}`)}
		if err := db.Create(&plugin).Error; err != nil {
			t.Fatalf("ensure apps plugin: %v", err)
		}
		return plugin
	}
	if err != nil {
		t.Fatalf("load apps plugin: %v", err)
	}
	// Force a plain (non-auto-install) manifest so the tool-gate is exercised via
	// explicit team grants, not the shipped plugin's auto-install path.
	if err := db.Model(&plugin).Updates(map[string]any{"status": model.PluginStatusActive, "manifest": model.RawJSON(`{}`)}).Error; err != nil {
		t.Fatalf("normalize apps plugin: %v", err)
	}
	return plugin
}

// installAppsPlugin grants the apps plugin to the agent's team (org-install +
// team_plugins) so it resolves into the agent's effective set.
func installAppsPlugin(t *testing.T, db *gorm.DB, orgID, teamID uuid.UUID) {
	t.Helper()
	plugin := ensureAppsPlugin(t, db)
	orgInstall := model.OrgPluginInstall{OrgID: orgID, PluginID: plugin.ID}
	if err := db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).
		FirstOrCreate(&orgInstall).Error; err != nil {
		t.Fatalf("org-install apps plugin: %v", err)
	}
	grant := model.TeamPlugin{OrgID: orgID, TeamID: teamID, PluginID: plugin.ID}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("grant apps plugin to team: %v", err)
	}
	t.Cleanup(func() {
		db.Where("team_id = ? AND plugin_id = ?", teamID, plugin.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).Delete(&model.OrgPluginInstall{})
	})
}

func appAgentToken(orgID, agentID uuid.UUID) *model.Token {
	return &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}
}

// connectAppToolsClient registers the tool group on an in-memory MCP server
// and returns a connected client session.
func connectAppToolsClient(t *testing.T, ctx context.Context, svc *Service, token *model.Token) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	NewToolsFunc(svc)(server, token)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "apps-mcp-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// seedAppsSession creates a session for the harness agent in the given
// channel, mirroring the Rust runtime's session context. Deleted with the org.
func seedAppsSession(t *testing.T, h *appsTestHarness, channelID uuid.UUID) model.Session {
	t.Helper()
	session := model.Session{ID: uuid.New(), OrgID: h.org.ID, ChannelID: channelID, AgentID: h.agent.ID}
	if err := h.db.Create(&session).Error; err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return session
}

// setupAppTools builds the standard MCP fixture: harness, plugin install for
// the harness agent, a session in the primary channel, and a connected client.
func setupAppTools(t *testing.T) (*appsTestHarness, *mcp.ClientSession, model.Session) {
	t.Helper()
	h := newAppsTestHarness(t)
	installAppsPlugin(t, h.db, h.org.ID, h.team.ID)
	session := seedAppsSession(t, h, h.channel.ID)
	client := connectAppToolsClient(t, context.Background(), h.svc, appAgentToken(h.org.ID, h.agent.ID))
	return h, client, session
}

func appToolResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// callAppToolRaw invokes a tool with the session injected (like the runtime
// proxy does) and returns the raw result.
func callAppToolRaw(t *testing.T, client *mcp.ClientSession, session model.Session, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	if _, ok := args["_hivy_session_id"]; !ok {
		args["_hivy_session_id"] = session.ID.String()
	}
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return result
}

// callAppTool invokes a tool expecting success and decodes the JSON reply.
func callAppTool(t *testing.T, client *mcp.ClientSession, session model.Session, name string, args map[string]any) map[string]any {
	t.Helper()
	result := callAppToolRaw(t, client, session, name, args)
	if result.IsError {
		t.Fatalf("%s returned error: %s", name, appToolResultText(result))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(appToolResultText(result)), &out); err != nil {
		t.Fatalf("decode %s response %q: %v", name, appToolResultText(result), err)
	}
	return out
}

// assertAppToolError invokes a tool expecting an IsError result (never a
// transport error) whose text contains want.
func assertAppToolError(t *testing.T, client *mcp.ClientSession, session model.Session, name string, args map[string]any, want string) {
	t.Helper()
	result := callAppToolRaw(t, client, session, name, args)
	if !result.IsError || !strings.Contains(appToolResultText(result), want) {
		t.Fatalf("%s IsError = %v text %q, want error containing %q", name, result.IsError, appToolResultText(result), want)
	}
}

func listAppToolNames(t *testing.T, ctx context.Context, client *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	tools, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	return byName
}

func appToolNameList(tools map[string]*mcp.Tool) []string {
	out := make([]string, 0, len(tools))
	for name := range tools {
		out = append(out, name)
	}
	return out
}
