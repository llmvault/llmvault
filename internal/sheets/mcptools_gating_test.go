package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// TestSheetToolsGating pins the plugin-install gate: the tool group registers
// only for agent-proxy tokens whose agent has the sheets plugin installed in
// the caller's org.
func TestSheetToolsGating(t *testing.T) {
	ctx := context.Background()
	db := connectSheetsTestDB(t)
	fixture := seedSheetsFixture(t, db)
	ensureSheetsPlugin(t, db)

	// No agent_plugin_installs row → nothing registers.
	client := connectSheetToolsClient(t, ctx, fixture.svc, sheetAgentToken(fixture.org.ID, fixture.agent.ID))
	if tools := listSheetToolNames(t, ctx, client); len(tools) != 0 {
		t.Fatalf("tools registered without plugin install: %v", toolNameList(tools))
	}

	// Non-agent-proxy token → nothing registers even with an install.
	installSheetsPlugin(t, db, fixture.org.ID, fixture.agent.ID)
	plainToken := &model.Token{OrgID: fixture.org.ID, Meta: model.JSON{}}
	client = connectSheetToolsClient(t, ctx, fixture.svc, plainToken)
	if tools := listSheetToolNames(t, ctx, client); len(tools) != 0 {
		t.Fatalf("tools registered for non-agent-proxy token: %v", toolNameList(tools))
	}

	// Token org mismatching the agent's org → nothing registers (the install
	// belongs to fixture.org; the token claims otherOrg).
	client = connectSheetToolsClient(t, ctx, fixture.svc, sheetAgentToken(fixture.otherOrg.ID, fixture.agent.ID))
	if tools := listSheetToolNames(t, ctx, client); len(tools) != 0 {
		t.Fatalf("tools registered across orgs: %v", toolNameList(tools))
	}

	// Installed + agent proxy → all 8 tools register.
	client = connectSheetToolsClient(t, ctx, fixture.svc, sheetAgentToken(fixture.org.ID, fixture.agent.ID))
	tools := listSheetToolNames(t, ctx, client)
	if len(tools) != len(sheetToolNames) {
		t.Fatalf("registered %d tools, want %d: %v", len(tools), len(sheetToolNames), toolNameList(tools))
	}
	for _, name := range sheetToolNames {
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
		if strings.Contains(string(schema), "_hivy_session_id") {
			t.Fatalf("tool %s exposes _hivy_session_id in schema: %s", name, schema)
		}
		if !strings.Contains(string(schema), `"additionalProperties":false`) {
			t.Fatalf("tool %s schema is missing additionalProperties:false: %s", name, schema)
		}
	}
}

func toolNameList(tools map[string]*mcp.Tool) []string {
	out := make([]string, 0, len(tools))
	for name := range tools {
		out = append(out, name)
	}
	return out
}
