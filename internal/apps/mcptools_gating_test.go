package apps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// TestAppToolsGating pins the plugin-install gate: the tool group registers
// only for agent-proxy tokens whose agent has the apps plugin installed in
// the caller's org.
func TestAppToolsGating(t *testing.T) {
	ctx := context.Background()
	h := newAppsTestHarness(t)
	ensureAppsPlugin(t, h.db)

	// No agent_plugin_installs row → nothing registers.
	client := connectAppToolsClient(t, ctx, h.svc, appAgentToken(h.org.ID, h.agent.ID))
	if tools := listAppToolNames(t, ctx, client); len(tools) != 0 {
		t.Fatalf("tools registered without plugin install: %v", appToolNameList(tools))
	}

	// Non-agent-proxy token → nothing registers even with an install.
	installAppsPlugin(t, h.db, h.org.ID, h.agent.ID)
	plainToken := &model.Token{OrgID: h.org.ID, Meta: model.JSON{}}
	client = connectAppToolsClient(t, ctx, h.svc, plainToken)
	if tools := listAppToolNames(t, ctx, client); len(tools) != 0 {
		t.Fatalf("tools registered for non-agent-proxy token: %v", appToolNameList(tools))
	}

	// Token org mismatching the agent's org → nothing registers (the install
	// belongs to h.org; the token claims otherOrg).
	otherOrg := model.Org{ID: uuid.New(), Name: "apps-mcp-other-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := h.db.Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.Org{}, "id = ?", otherOrg.ID) })
	client = connectAppToolsClient(t, ctx, h.svc, appAgentToken(otherOrg.ID, h.agent.ID))
	if tools := listAppToolNames(t, ctx, client); len(tools) != 0 {
		t.Fatalf("tools registered across orgs: %v", appToolNameList(tools))
	}

	// Installed + agent proxy → all 5 tools register with instructive
	// descriptions and closed schemas that never leak the injected keys.
	client = connectAppToolsClient(t, ctx, h.svc, appAgentToken(h.org.ID, h.agent.ID))
	tools := listAppToolNames(t, ctx, client)
	if len(tools) != len(appToolNames) {
		t.Fatalf("registered %d tools, want %d: %v", len(tools), len(appToolNames), appToolNameList(tools))
	}
	for _, name := range appToolNames {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("tool %s not registered", name)
		}
		if len(tool.Description) < 100 {
			t.Fatalf("tool %s has weak description %q", name, tool.Description)
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", name, err)
		}
		if strings.Contains(string(schema), "_hivy_session_id") || strings.Contains(string(schema), "_hivy_actor_user_id") {
			t.Fatalf("tool %s exposes an injected key in schema: %s", name, schema)
		}
		if !strings.Contains(string(schema), `"additionalProperties":false`) {
			t.Fatalf("tool %s schema is missing additionalProperties:false: %s", name, schema)
		}
	}
}

// TestAppCreateTool covers the create happy path (session-derived channel,
// agent + session attribution) and its guardrails.
func TestAppCreateTool(t *testing.T) {
	h, client, session := setupAppTools(t)

	out := callAppTool(t, client, session, toolAppCreate, map[string]any{
		"name":        "Leads Manager",
		"description": "Browse and qualify leads.",
		"sheet_id":    h.sheet.ID.String(),
	})
	if out["slug"] != "leads-manager" || out["status"] != string(model.AppStatusDraft) {
		t.Fatalf("app_create response = %v", out)
	}
	appID := mustAppUUID(t, out["app_id"])

	var app model.App
	if err := h.db.First(&app, "id = ?", appID).Error; err != nil {
		t.Fatalf("load created app: %v", err)
	}
	if app.ChannelID != h.channel.ID || app.SheetID != h.sheet.ID {
		t.Fatalf("app channel/sheet = %s/%s, want %s/%s", app.ChannelID, app.SheetID, h.channel.ID, h.sheet.ID)
	}
	if app.CreatedByAgentID == nil || *app.CreatedByAgentID != h.agent.ID {
		t.Fatalf("created_by_agent_id = %v, want %s", app.CreatedByAgentID, h.agent.ID)
	}
	if app.SourceSessionID == nil || *app.SourceSessionID != session.ID {
		t.Fatalf("source_session_id = %v, want %s", app.SourceSessionID, session.ID)
	}

	// The bound sheet must live in the session's channel.
	assertAppToolError(t, client, session, toolAppCreate, map[string]any{
		"name":     "Cross Channel",
		"sheet_id": h.otherSheet.ID.String(),
	}, "sheet does not belong to this channel")

	// A sheet that does not exist reads the same as one out of scope.
	assertAppToolError(t, client, session, toolAppCreate, map[string]any{
		"name":     "Ghost Sheet",
		"sheet_id": uuid.NewString(),
	}, "sheet not found in this channel")

	// No session → apps are channel-scoped, so the call is refused.
	assertAppToolError(t, client, session, toolAppCreate, map[string]any{
		"name":             "No Session",
		"sheet_id":         h.sheet.ID.String(),
		"_hivy_session_id": "",
	}, "must be called from within a session")
}

func mustAppUUID(t *testing.T, raw any) uuid.UUID {
	t.Helper()
	text, _ := raw.(string)
	id, err := uuid.Parse(text)
	if err != nil {
		t.Fatalf("parse uuid %v: %v", raw, err)
	}
	return id
}
