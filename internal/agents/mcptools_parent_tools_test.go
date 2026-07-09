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

func denySet(t *testing.T, f *model.ToolFilter) map[string]bool {
	t.Helper()
	if f == nil {
		t.Fatalf("expected a deny-list filter, got nil")
	}
	if len(f.Allow) != 0 {
		t.Fatalf("parent filter must not use an allow-list, got Allow=%v", f.Allow)
	}
	out := map[string]bool{}
	for _, d := range f.Deny {
		out[d] = true
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
// incident: an agent that picks only the read-only skill tools must still get
// every baseline runtime tool, and its MCP deny-list must NOT lock out the
// read-only floor (skills_list/skill_view/list_channels).
func TestCreateAgent_SkillOnlyPick_GrantsBaseline(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}

	res, _ := handleCreateAgent(context.Background(), deps, token, team.ID, "https://app.test", createAgentArgs{
		Name:  "Skill Only",
		Tools: []string{"skills_list", "skill_view"},
	})
	obj := builderResultJSON(t, res)["agent"].(map[string]any)
	stored := loadAgentByID(t, db, uuid.MustParse(obj["id"].(string)))

	assertBaselineGranted(t, stored.Tools)

	deny := denySet(t, stored.McpToolFilter)
	for _, floor := range model.ReadOnlyMCPToolFloor {
		if deny[floor] {
			t.Fatalf("deny-list must not deny read-only floor %q: %v", floor, stored.McpToolFilter.Deny)
		}
	}
	for _, want := range []string{"web_search", "cron", "generate_image"} {
		if !deny[want] {
			t.Fatalf("deny-list must deny unpicked capability %q: %v", want, stored.McpToolFilter.Deny)
		}
	}
}

// TestCreateAgent_PicksTwoMCP_DenyRest checks the deny-list excludes the picked
// capabilities and includes the rest.
func TestCreateAgent_PicksTwoMCP_DenyRest(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	deps := noopDeps(db)
	token := &model.Token{OrgID: org.ID}

	res, _ := handleCreateAgent(context.Background(), deps, token, team.ID, "https://app.test", createAgentArgs{
		Name:  "Two MCP",
		Tools: []string{"cron", "web_search"},
	})
	obj := builderResultJSON(t, res)["agent"].(map[string]any)
	stored := loadAgentByID(t, db, uuid.MustParse(obj["id"].(string)))

	assertBaselineGranted(t, stored.Tools)
	deny := denySet(t, stored.McpToolFilter)
	if deny["cron"] || deny["web_search"] {
		t.Fatalf("deny-list must not deny picked cron/web_search: %v", stored.McpToolFilter.Deny)
	}
	if !deny["generate_image"] || !deny["create_http_trigger"] {
		t.Fatalf("deny-list must deny unpicked generate_image/create_http_trigger: %v", stored.McpToolFilter.Deny)
	}
	// The echo reports only the picked parent-assignable capabilities.
	echo := agentToolStrings(t, obj, "tools")
	sort.Strings(echo)
	if !reflect.DeepEqual(echo, []string{"cron", "web_search"}) {
		t.Fatalf("parent echo = %v, want [cron web_search]", echo)
	}
}

// TestCreateAgent_AllMCPPicked_NilFilter checks that when every
// parent-assignable MCP tool is picked, the filter is nil (nothing to deny).
func TestCreateAgent_AllMCPPicked_NilFilter(t *testing.T) {
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
	if stored.McpToolFilter != nil {
		t.Fatalf("filter must be nil when all parent-assignable MCP tools are picked, got: %#v", stored.McpToolFilter)
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
			{Name: "Explicit", Tools: []string{"read_file", "grep"}},
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

	// Explicit runtime picks stored verbatim; no MCP filter.
	explicit := byName["Explicit"]
	if len(explicit.Tools) != 2 || explicit.Tools["read_file"] != true || explicit.Tools["grep"] != true {
		t.Fatalf("Explicit sub tools = %#v, want exactly read_file+grep", explicit.Tools)
	}
	if explicit.McpToolFilter != nil {
		t.Fatalf("Explicit sub must have nil filter, got: %#v", explicit.McpToolFilter)
	}

	// Empty picks default to the read-only sandbox set.
	empty := byName["Empty"]
	wantReadOnly := map[string]bool{"read_file": true, "glob": true, "grep": true, "file_search": true}
	if len(empty.Tools) != len(wantReadOnly) {
		t.Fatalf("Empty sub tools = %#v, want read-only default", empty.Tools)
	}
	for id := range wantReadOnly {
		if empty.Tools[id] != true {
			t.Fatalf("Empty sub missing read-only default %q: %#v", id, empty.Tools)
		}
	}
	if empty.McpToolFilter != nil {
		t.Fatalf("Empty sub must inherit (nil filter), got: %#v", empty.McpToolFilter)
	}

	// A single MCP pick keeps allow-list semantics and unions the read-only floor.
	web := byName["WebPick"]
	if web.McpToolFilter == nil {
		t.Fatalf("WebPick sub must have an allow-list filter")
	}
	allow := append([]string(nil), web.McpToolFilter.Allow...)
	sort.Strings(allow)
	want := []string{"list_channels", "skill_view", "skills_list", "web_search"}
	if !reflect.DeepEqual(allow, want) {
		t.Fatalf("WebPick allow = %v, want %v", allow, want)
	}
	if len(web.Tools) != 0 {
		t.Fatalf("WebPick sub runtime tools must be empty, got: %#v", web.Tools)
	}
}

// TestUpdateAgent_ToolsReplacementKeepsBaselineAndSubagentTask verifies a
// tools-only patch re-grants baseline, recomputes the deny-list, and keeps
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
	deny := denySet(t, stored.McpToolFilter)
	if deny["generate_image"] {
		t.Fatalf("deny-list must not deny the newly picked generate_image: %v", stored.McpToolFilter.Deny)
	}
	if !deny["web_search"] {
		t.Fatalf("replaced tools must now deny web_search (no longer picked): %v", stored.McpToolFilter.Deny)
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

	newTools := []string{"cron"}
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
