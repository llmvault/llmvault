package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// sheetToolNames is the full expected tool group, in registration order.
var sheetToolNames = []string{
	toolSheetCreate, toolSheetList, toolSheetDescribe, toolSheetManage,
	toolRowsQuery, toolRowsWrite, toolSheetImportCSV, toolSheetOperations,
}

// ensureSheetsPlugin returns the active plugin row with slug "sheets",
// creating it when the test DB has not synced global plugins. The row is
// intentionally NOT cleaned up: the slug is unique and shared across tests.
func ensureSheetsPlugin(t *testing.T, db *gorm.DB) model.Plugin {
	t.Helper()
	plugin := model.Plugin{
		Slug:   SheetsPluginSlug,
		Name:   "Sheets",
		Status: model.PluginStatusActive,
	}
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "slug"}},
		DoUpdates: clause.Assignments(map[string]any{"status": model.PluginStatusActive}),
	}).Create(&plugin).Error
	if err != nil {
		t.Fatalf("ensure sheets plugin: %v", err)
	}
	if err := db.Where("slug = ?", SheetsPluginSlug).First(&plugin).Error; err != nil {
		t.Fatalf("load sheets plugin: %v", err)
	}
	return plugin
}

// installSheetsPlugin creates the agent_plugin_installs row that gates the
// tool group for one agent.
func installSheetsPlugin(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) {
	t.Helper()
	plugin := ensureSheetsPlugin(t, db)
	install := model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: plugin.ID}
	if err := db.Create(&install).Error; err != nil {
		t.Fatalf("install sheets plugin for agent: %v", err)
	}
	t.Cleanup(func() {
		db.Where("agent_id = ? AND plugin_id = ?", agentID, plugin.ID).Delete(&model.AgentPluginInstall{})
	})
}

func sheetAgentToken(orgID, agentID uuid.UUID) *model.Token {
	return &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentID.String(),
		},
	}
}

// connectSheetToolsClient registers the tool group on an in-memory MCP
// server and returns a connected client session.
func connectSheetToolsClient(t *testing.T, ctx context.Context, svc *Service, token *model.Token) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "hivy-test", Version: "v1"}, nil)
	NewToolsFunc(svc)(server, token)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "sheets-mcp-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// setupSheetTools seeds the standard sheets fixture, installs the plugin for
// the fixture agent, and connects a client.
func setupSheetTools(t *testing.T) (*sheetsFixture, *mcp.ClientSession) {
	t.Helper()
	db := connectSheetsTestDB(t)
	fixture := seedSheetsFixture(t, db)
	installSheetsPlugin(t, db, fixture.org.ID, fixture.agent.ID)
	client := connectSheetToolsClient(t, context.Background(), fixture.svc, sheetAgentToken(fixture.org.ID, fixture.agent.ID))
	return fixture, client
}

func sheetToolText(result *mcp.CallToolResult) string {
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

// callSheetTool invokes a tool expecting success and decodes the JSON reply.
func callSheetTool(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned error: %s", name, sheetToolText(result))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(sheetToolText(result)), &out); err != nil {
		t.Fatalf("decode %s response %q: %v", name, sheetToolText(result), err)
	}
	return out
}

// callSheetToolJSON invokes a tool with a raw JSON argument payload — used by
// the SKILL.md contract tests, which must feed the documented payloads
// byte-for-byte (module placeholder IDs).
func callSheetToolJSON(t *testing.T, ctx context.Context, client *mcp.ClientSession, name, payload string) map[string]any {
	t.Helper()
	var args map[string]any
	if err := json.Unmarshal([]byte(payload), &args); err != nil {
		t.Fatalf("skill payload for %s is not valid JSON: %v\n%s", name, err, payload)
	}
	return callSheetTool(t, ctx, client, name, args)
}

// assertSheetToolError invokes a tool expecting an IsError result (never a
// transport error) whose text contains want.
func assertSheetToolError(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: transport error instead of IsError result: %v", name, err)
	}
	if !result.IsError || !strings.Contains(sheetToolText(result), want) {
		t.Fatalf("%s IsError = %v text %q, want error containing %q", name, result.IsError, sheetToolText(result), want)
	}
}

func listSheetToolNames(t *testing.T, ctx context.Context, client *mcp.ClientSession) map[string]*mcp.Tool {
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

func responseRows(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["rows"].([]any)
	if !ok {
		t.Fatalf("response has no rows array: %#v", out)
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("row is not an object: %#v", item)
		}
		rows = append(rows, row)
	}
	return rows
}

func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", raw, err)
	}
	return id
}

func rowIDs(t *testing.T, out map[string]any) []string {
	t.Helper()
	rows := responseRows(t, out)
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		ids = append(ids, id)
	}
	return ids
}
