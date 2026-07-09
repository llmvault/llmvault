package sheets

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
	// The sheets plugin is a global plugin (org_id NULL). Since migration 57 the
	// slug uniqueness is a partial index (WHERE org_id IS NULL), so a bare
	// ON CONFLICT(slug) no longer matches — find-or-create instead.
	var plugin model.Plugin
	err := db.Where("slug = ? AND org_id IS NULL", SheetsPluginSlug).First(&plugin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		plugin = model.Plugin{Slug: SheetsPluginSlug, Name: "Sheets", Status: model.PluginStatusActive, Manifest: model.RawJSON(`{}`)}
		if err := db.Create(&plugin).Error; err != nil {
			t.Fatalf("ensure sheets plugin: %v", err)
		}
		return plugin
	}
	if err != nil {
		t.Fatalf("load sheets plugin: %v", err)
	}
	// Force a plain (non-auto-install) manifest so the tool-gate is exercised via
	// explicit team grants, not the shipped plugin's auto-install path.
	if err := db.Model(&plugin).Updates(map[string]any{"status": model.PluginStatusActive, "manifest": model.RawJSON(`{}`)}).Error; err != nil {
		t.Fatalf("normalize sheets plugin: %v", err)
	}
	return plugin
}

// installSheetsPlugin grants the sheets plugin to the agent's team (org-install
// + team_plugins) so it resolves into the agent's effective set.
func installSheetsPlugin(t *testing.T, db *gorm.DB, orgID, teamID uuid.UUID) {
	t.Helper()
	plugin := ensureSheetsPlugin(t, db)
	orgInstall := model.OrgPluginInstall{OrgID: orgID, PluginID: plugin.ID}
	if err := db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).
		FirstOrCreate(&orgInstall).Error; err != nil {
		t.Fatalf("org-install sheets plugin: %v", err)
	}
	grant := model.TeamPlugin{OrgID: orgID, TeamID: teamID, PluginID: plugin.ID}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("grant sheets plugin to team: %v", err)
	}
	t.Cleanup(func() {
		db.Where("team_id = ? AND plugin_id = ?", teamID, plugin.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, plugin.ID).Delete(&model.OrgPluginInstall{})
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
	installSheetsPlugin(t, db, fixture.org.ID, fixture.team.ID)
	client := connectSheetToolsClient(t, context.Background(), fixture.svc, sheetAgentToken(fixture.org.ID, fixture.agent.ID))
	return fixture, client
}

// sessionCtxKey carries the runtime-injected _hivy_session_id in tests, mirroring
// the Rust proxy that injects it into every sheets tool call. The call helpers
// read it and stamp it onto tool args, so tests do not repeat it per call.
type sessionCtxKey struct{}

// toolCtx returns a context that stamps the fixture session onto tool calls made
// through the sheets MCP call helpers.
func (f *sheetsFixture) toolCtx() context.Context {
	return context.WithValue(context.Background(), sessionCtxKey{}, f.session.ID.String())
}

// injectSheetSession adds the context's _hivy_session_id to args when the caller
// did not set one explicitly, replicating the runtime proxy's injection.
func injectSheetSession(ctx context.Context, args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	if _, ok := args["_hivy_session_id"]; ok {
		return args
	}
	if id, ok := ctx.Value(sessionCtxKey{}).(string); ok && id != "" {
		args["_hivy_session_id"] = id
	}
	return args
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
	args = injectSheetSession(ctx, args)
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
	args = injectSheetSession(ctx, args)
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
