package agents

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/model"
	"gorm.io/gorm"
)

// loadAgentByID loads a stored agent row (parent or sub) for assertions.
func loadAgentByID(t *testing.T, db *gorm.DB, id uuid.UUID) model.Agent {
	t.Helper()
	var a model.Agent
	if err := db.Where("id = ?", id).First(&a).Error; err != nil {
		t.Fatalf("load agent %s: %v", id, err)
	}
	return a
}

func allowSet(t *testing.T, f *model.ToolFilter) map[string]bool {
	t.Helper()
	if f == nil {
		t.Fatalf("expected an allow-list filter, got nil")
	}
	if len(f.Deny) != 0 {
		t.Fatalf("parent filter must not use deny rules, got Deny=%v", f.Deny)
	}
	out := map[string]bool{}
	for _, allowed := range f.Allow {
		out[allowed] = true
	}
	return out
}

func assertBaselineGranted(t *testing.T, tools model.JSON) {
	t.Helper()
	for _, id := range model.BaselineRuntimeToolIDs {
		if tools[id] != true {
			t.Fatalf("baseline runtime tool %q not granted: %#v", id, tools)
		}
	}
}

// TestCreateAgent_SkillOnlyPick_GrantsBaseline is the direct regression for the
// incident: an agent with no optional MCP picks must still get every baseline
// runtime tool and only the universal skill_view MCP capability.
func TestCreateAgent_NoMCPPicks_GrantsBaseline(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}

	res, _ := handleCreateAgent(context.Background(), deps, token, team.ID, "https://app.test", createAgentArgs{
		Name: "No MCP tools",
	})
	obj := builderResultJSON(t, res)["agent"].(map[string]any)
	stored := loadAgentByID(t, db, uuid.MustParse(obj["id"].(string)))

	assertBaselineGranted(t, stored.Tools)

	allow := allowSet(t, stored.McpToolFilter)
	for _, floor := range model.ReadOnlyMCPToolFloor {
		if allow[floor] {
			t.Fatalf("stored parent filter must not persist universal tool %q: %v", floor, stored.McpToolFilter.Allow)
		}
	}
	if len(allow) != 0 {
		t.Fatalf("allow list = %v, want no optional MCP tools", stored.McpToolFilter.Allow)
	}
}

// TestCreateAgent_PicksTwoMCP_AllowListsOnlyThoseCapabilities verifies that
// connection capability and future MCP tools cannot leak through an unbounded deny-list.
func TestCreateAgent_PicksTwoMCP_AllowListsOnlyThoseCapabilities(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}

	res, _ := handleCreateAgent(context.Background(), deps, token, team.ID, "https://app.test", createAgentArgs{
		Name:  "Two MCP",
		Tools: []string{"web_crawl", "web_search"},
	})
	obj := builderResultJSON(t, res)["agent"].(map[string]any)
	stored := loadAgentByID(t, db, uuid.MustParse(obj["id"].(string)))

	assertBaselineGranted(t, stored.Tools)
	allow := allowSet(t, stored.McpToolFilter)
	for _, id := range []string{"web_crawl", "web_search"} {
		if !allow[id] {
			t.Fatalf("allow list must grant picked %q: %v", id, stored.McpToolFilter.Allow)
		}
	}
	if allow["generate_image"] || allow["sheet_list"] {
		t.Fatalf("allow list leaked an unpicked capability: %v", stored.McpToolFilter.Allow)
	}
	// The echo reports only the picked parent-assignable capabilities.
	echo := agentToolStrings(t, obj, "tools")
	sort.Strings(echo)
	if !reflect.DeepEqual(echo, []string{"web_crawl", "web_search"}) {
		t.Fatalf("parent echo = %v, want [web_crawl web_search]", echo)
	}
}

// TestCreateAgent_AllMCPPicked_StillStoresAnAllowList proves that even a full
// optional selection cannot turn into the runtime's nil/allow-all semantics.
func TestCreateAgent_AllMCPPicked_StillStoresAnAllowList(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}

	res, _ := handleCreateAgent(context.Background(), deps, token, team.ID, "https://app.test", createAgentArgs{
		Name:  "All MCP",
		Tools: append([]string{"lsp"}, parentAssignableMCPTools()...),
	})
	obj := builderResultJSON(t, res)["agent"].(map[string]any)
	stored := loadAgentByID(t, db, uuid.MustParse(obj["id"].(string)))

	assertBaselineGranted(t, stored.Tools)
	allow := allowSet(t, stored.McpToolFilter)
	for _, id := range parentAssignableMCPTools() {
		if !allow[id] {
			t.Fatalf("allow list missing picked tool %q: %#v", id, stored.McpToolFilter)
		}
	}
	if stored.Tools["lsp"] != true {
		t.Fatalf("optional runtime tool lsp not granted: %#v", stored.Tools)
	}
}

// TestCreateAgent_SubAgentToolShapes covers the sub-agent tool routing:
// subagent_task on the parent, explicit runtime picks, the read-only default,
// and floor-augmented allow lists.
func TestCreateAgent_SubAgentToolShapes(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}

	res, _ := handleCreateAgent(context.Background(), deps, token, team.ID, "https://app.test", createAgentArgs{
		Name: "Coordinator",
		SubAgents: []subAgentToolArgs{
			{Name: "Explicit", Tools: []string{"read_file", "bash"}},
			{Name: "Empty"},
			{Name: "WebPick", Tools: []string{"web_search"}},
		},
	})
	obj := builderResultJSON(t, res)["agent"].(map[string]any)
	parent := loadAgentByID(t, db, uuid.MustParse(obj["id"].(string)))
	if parent.Tools["subagent_task"] != true {
		t.Fatalf("parent must gain subagent_task: %#v", parent.Tools)
	}

	var subs []model.Agent
	if err := db.Where("parent_agent_id = ? AND type = ?", parent.ID, model.AgentTypeSubAgent).Find(&subs).Error; err != nil {
		t.Fatalf("load subs: %v", err)
	}
	byName := map[string]model.Agent{}
	for _, s := range subs {
		byName[s.Name] = s
	}

	// Explicit runtime picks stored verbatim; MCP tools are floored to the
	// universal skill_view tool rather than inheriting the parent's grants.
	explicit := byName["Explicit"]
	if len(explicit.Tools) != 2 || explicit.Tools["read_file"] != true || explicit.Tools["bash"] != true {
		t.Fatalf("Explicit sub tools = %#v, want exactly read_file+bash", explicit.Tools)
	}
	assertReadOnlyMCPFloor(t, "Explicit", explicit.McpToolFilter)

	// Empty picks default to read_file plus the universal MCP floor.
	empty := byName["Empty"]
	wantReadOnly := map[string]bool{"read_file": true}
	if len(empty.Tools) != len(wantReadOnly) {
		t.Fatalf("Empty sub tools = %#v, want read-only default", empty.Tools)
	}
	for id := range wantReadOnly {
		if empty.Tools[id] != true {
			t.Fatalf("Empty sub missing read-only default %q: %#v", id, empty.Tools)
		}
	}
	assertReadOnlyMCPFloor(t, "Empty", empty.McpToolFilter)

	// A single MCP pick keeps allow-list semantics and unions the universal floor.
	web := byName["WebPick"]
	if web.McpToolFilter == nil {
		t.Fatalf("WebPick sub must have an allow-list filter")
	}
	allow := append([]string(nil), web.McpToolFilter.Allow...)
	sort.Strings(allow)
	want := []string{"skill_view", "web_search"}
	if !reflect.DeepEqual(allow, want) {
		t.Fatalf("WebPick allow = %v, want %v", allow, want)
	}
	if len(web.Tools) != 0 {
		t.Fatalf("WebPick sub runtime tools must be empty, got: %#v", web.Tools)
	}
}

func assertReadOnlyMCPFloor(t *testing.T, name string, f *model.ToolFilter) {
	t.Helper()
	if f == nil {
		t.Fatalf("%s sub must be floored to the read-only MCP set, got nil (inherit-all)", name)
	}
	if len(f.Deny) != 0 {
		t.Fatalf("%s sub floor must not carry a deny list, got: %#v", name, f.Deny)
	}
	allow := append([]string(nil), f.Allow...)
	sort.Strings(allow)
	want := append([]string(nil), model.ReadOnlyMCPToolFloor...)
	sort.Strings(want)
	if !reflect.DeepEqual(allow, want) {
		t.Fatalf("%s sub allow = %v, want read-only floor %v", name, allow, want)
	}
}

// TestUpdateAgent_ToolsReplacementKeepsBaselineAndSubagentTask verifies a
// tools-only patch re-grants baseline, replaces the allow-list, and keeps
// subagent_task while active sub-agent rows exist.
func TestUpdateAgent_ToolsReplacementKeepsBaselineAndSubagentTask(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}
	ctx := context.Background()

	createRes, _ := handleCreateAgent(ctx, deps, token, team.ID, "https://app.test", createAgentArgs{
		Name:      "Patchable",
		Tools:     []string{"web_search"},
		SubAgents: []subAgentToolArgs{{Name: "Worker", Tools: []string{"read_file"}}},
	})
	agentID := uuid.MustParse(builderResultJSON(t, createRes)["agent"].(map[string]any)["id"].(string))

	// Patch tools only (sub_agents untouched) -> subagent_task must survive
	// because the Worker sub-agent still exists.
	newTools := []string{"generate_image"}
	updRes, _ := handleUpdateAgent(ctx, deps, token, "https://app.test", updateAgentArgs{
		AgentID: agentID.String(),
		Tools:   &newTools,
	})
	builderResultJSON(t, updRes)
	stored := loadAgentByID(t, db, agentID)

	assertBaselineGranted(t, stored.Tools)
	if stored.Tools["subagent_task"] != true {
		t.Fatalf("tools-only update must keep subagent_task (sub-agent still present): %#v", stored.Tools)
	}
	allow := allowSet(t, stored.McpToolFilter)
	if !allow["generate_image"] || allow["web_search"] {
		t.Fatalf("replaced MCP allow list = %v, want only generate_image", stored.McpToolFilter.Allow)
	}
}

// TestUpdateAgent_ToolsWithoutSubAgents_DropsSubagentTask verifies that when an
// agent has no active sub-agent rows, a tools-only patch does not add
// subagent_task.
func TestUpdateAgent_ToolsWithoutSubAgents_DropsSubagentTask(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}
	ctx := context.Background()

	createRes, _ := handleCreateAgent(ctx, deps, token, team.ID, "https://app.test", createAgentArgs{
		Name:  "NoSubs",
		Tools: []string{"web_search"},
	})
	agentID := uuid.MustParse(builderResultJSON(t, createRes)["agent"].(map[string]any)["id"].(string))

	newTools := []string{"web_crawl"}
	updRes, _ := handleUpdateAgent(ctx, deps, token, "https://app.test", updateAgentArgs{
		AgentID: agentID.String(),
		Tools:   &newTools,
	})
	builderResultJSON(t, updRes)
	stored := loadAgentByID(t, db, agentID)

	assertBaselineGranted(t, stored.Tools)
	if _, ok := stored.Tools["subagent_task"]; ok {
		t.Fatalf("agent without sub-agents must not gain subagent_task: %#v", stored.Tools)
	}
}
