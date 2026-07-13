package agents

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/model"
)

func TestAgentBuilderSkillPayloadContract(t *testing.T) {
	ctx := context.Background()

	// 0. Every payload constant must appear VERBATIM in the shipped SKILL.md —
	// editing one side without the other fails here.
	raw, err := os.ReadFile("../../global/plugins/agent-builder/skills/agent-builder/SKILL.md")
	if err != nil {
		t.Fatalf("read agent-builder SKILL.md: %v", err)
	}
	skillText := string(raw)
	for name, payload := range map[string]string{
		"get_agent":          abPayloadGetAgent,
		"create_agent":       abPayloadCreate,
		"update_description": abPayloadUpdateDescription,
		"update_archive":     abPayloadUpdateArchive,
		"update_add_tool":    abPayloadUpdateAddTool,
		"update_model":       abPayloadUpdateModel,
	} {
		if !strings.Contains(skillText, payload) {
			t.Fatalf("payload %q is not present verbatim in SKILL.md — skill and contract test drifted", name)
		}
	}

	// 1. Strict schema decodes: every documented key exists on the args structs.
	assertBuilderStrictDecode(t, abPayloadGetAgent, &getAgentArgs{})
	assertBuilderStrictDecode(t, abPayloadCreate, &createAgentArgs{})
	assertBuilderStrictDecode(t, abPayloadUpdateDescription, &updateAgentArgs{})
	assertBuilderStrictDecode(t, abPayloadUpdateArchive, &updateAgentArgs{})
	assertBuilderStrictDecode(t, abPayloadUpdateAddTool, &updateAgentArgs{})
	assertBuilderStrictDecode(t, abPayloadUpdateModel, &updateAgentArgs{})

	// 2. Fixtures: org, an installed plugin publishing a skill, deps with a
	// model catalog so the documented model-change payload can execute.
	db := testDB(t)
	org := testOrg(t, db)
	team := testTeam(t, db, org.ID)
	skillSlug := "github-triage-" + uuid.NewString()[:8]
	plugin := seedInstalledPlugin(t, db, org.ID, "github", skillSlug)
	grantTeamPlugin(t, db, org.ID, team.ID, plugin.ID)
	deps := Deps{DB: db, DefaultModel: "deepseek-v4-flash", Models: []string{"deepseek-v4-flash", "test-model-alt"}}
	callingAgent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		TeamID:        team.ID,
		Name:          "Hivy",
		IsDefault:     true,
		SandboxImage:  model.SandboxImageDefault,
		SandboxSize:   model.DefaultHivyAgentSandboxSize,
		Model:         "deepseek-v4-flash",
		Status:        "active",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
	}
	if err := db.Create(&callingAgent).Error; err != nil {
		t.Fatalf("create calling Hivy: %v", err)
	}
	token := &model.Token{OrgID: org.ID, Meta: model.JSON{
		model.TokenMetaType:    model.TokenTypeAgentProxy,
		model.TokenMetaAgentID: callingAgent.ID.String(),
	}}
	const frontend = "https://app.test"

	replace := strings.NewReplacer(
		"github-triage", skillSlug,
		"github", plugin.Slug,
	).Replace

	// 3. The documented empty payloads for the two list tools.
	if res, _ := handleListAgents(ctx, db, token, ""); res == nil || res.IsError {
		t.Fatalf("list_agents with documented %s payload failed", abPayloadListEmpty)
	}
	var pluginsOut map[string]any
	{
		res, _ := handleListTeamPlugins(ctx, db, token, frontend)
		pluginsOut = builderResultJSON(t, res)
	}
	installedText, _ := json.Marshal(pluginsOut["installed"])
	for _, want := range []string{plugin.Slug, skillSlug, "install_url"} {
		if !strings.Contains(string(installedText), want) {
			t.Fatalf("list_team_plugins installed list missing %q: %s", want, installedText)
		}
	}

	// 4. create_agent with the documented Support Triage payload.
	var cArgs createAgentArgs
	if err := json.Unmarshal([]byte(replace(abPayloadCreate)), &cArgs); err != nil {
		t.Fatalf("create payload: %v", err)
	}
	createRes, _ := handleCreateAgent(ctx, deps, token, team.ID, frontend, cArgs)
	created := builderResultJSON(t, createRes)
	agentObj := created["agent"].(map[string]any)
	agentID := agentObj["id"].(string)
	if created["url"] == "" {
		t.Fatalf("create_agent response has no url")
	}
	tools := agentToolStrings(t, agentObj, "tools")
	// The parent tools array echoes only parent-assignable grants: the two picked
	// MCP capabilities. Baseline sandbox tools, universal skill_view, and the
	// auto-granted subagent_task are all applied
	// to the stored agent but intentionally omitted from the echo so the list
	// round-trips through the parent tools schema.
	for _, want := range []string{"web_search", "web_fetch"} {
		if !containsString(tools, want) {
			t.Fatalf("created agent tools missing picked capability %q: %v", want, tools)
		}
	}
	for _, forbidden := range []string{"skill_view", "subagent_task", "bash", "read_file"} {
		if containsString(tools, forbidden) {
			t.Fatalf("parent tools echo must omit auto-granted %q: %v", forbidden, tools)
		}
	}
	// The stored agent must still have the baseline sandbox tools and subagent_task
	// (auto-added because it has a sub-agent), and its MCP filter must be the
	// exact explicit list the caller selected.
	{
		var stored model.Agent
		if err := db.Where("id = ?", agentID).First(&stored).Error; err != nil {
			t.Fatalf("load stored agent: %v", err)
		}
		for _, id := range model.BaselineRuntimeToolIDs {
			if stored.Tools[id] != true {
				t.Fatalf("stored agent missing baseline runtime tool %q: %#v", id, stored.Tools)
			}
		}
		if stored.Tools["subagent_task"] != true {
			t.Fatalf("stored agent missing auto-granted subagent_task: %#v", stored.Tools)
		}
		if stored.McpToolFilter == nil || len(stored.McpToolFilter.Deny) != 0 {
			t.Fatalf("parent MCP filter must be an allow-list, got: %#v", stored.McpToolFilter)
		}
		allowed := map[string]bool{}
		for _, id := range stored.McpToolFilter.Allow {
			allowed[id] = true
		}
		for _, picked := range []string{"web_search", "web_fetch"} {
			if !allowed[picked] {
				t.Fatalf("allow-list must grant picked capability %q: %v", picked, stored.McpToolFilter.Allow)
			}
		}
		if allowed["web_crawl"] || allowed["generate_image"] || allowed["sheet_list"] {
			t.Fatalf("allow-list leaked an unpicked capability: %v", stored.McpToolFilter.Allow)
		}
	}
	if subs := agentObj["sub_agents"].([]any); len(subs) != 1 || subs[0].(map[string]any)["name"] != "Responder" {
		t.Fatalf("created agent sub_agents mismatch: %#v", agentObj["sub_agents"])
	}
	if !containsString(agentToolStrings(t, agentObj, "plugins"), plugin.Slug) {
		t.Fatalf("created agent plugins missing %q", plugin.Slug)
	}
	if !containsString(agentToolStrings(t, agentObj, "skills"), skillSlug) {
		t.Fatalf("created agent skills missing %q", skillSlug)
	}

	idReplace := strings.NewReplacer("7c9e6679-…", agentID).Replace

	// 5. get_agent with the documented payload.
	var gArgs getAgentArgs
	if err := json.Unmarshal([]byte(idReplace(abPayloadGetAgent)), &gArgs); err != nil {
		t.Fatalf("get payload: %v", err)
	}
	getRes, _ := handleGetAgent(ctx, db, token, frontend, "", gArgs)
	builderResultJSON(t, getRes)

	// 6-8. The documented updates: description patch, add-one-tool (full list
	// resent — nothing wiped), and the model change.
	runUpdate := func(payload string) map[string]any {
		t.Helper()
		var uArgs updateAgentArgs
		if err := json.Unmarshal([]byte(payload), &uArgs); err != nil {
			t.Fatalf("update payload: %v\n%s", err, payload)
		}
		res, _ := handleUpdateAgent(ctx, deps, token, frontend, uArgs)
		return builderResultJSON(t, res)
	}
	runUpdate(idReplace(abPayloadUpdateDescription))
	afterAdd := runUpdate(idReplace(abPayloadUpdateAddTool))
	addedTools := agentToolStrings(t, afterAdd["agent"].(map[string]any), "tools")
	// The resent full list adds generate_image while keeping web_search/web_fetch.
	// subagent_task stays granted on the stored agent (sub-agent still present) but
	// is not echoed for a parent.
	for _, want := range []string{"web_search", "web_fetch", "generate_image"} {
		if !containsString(addedTools, want) {
			t.Fatalf("add-one-tool update lost %q: %v", want, addedTools)
		}
	}
	if containsString(addedTools, "subagent_task") {
		t.Fatalf("parent tools echo must omit subagent_task: %v", addedTools)
	}
	{
		var stored model.Agent
		if err := db.Where("id = ?", agentID).First(&stored).Error; err != nil {
			t.Fatalf("load stored agent after add-tool update: %v", err)
		}
		if stored.Tools["subagent_task"] != true {
			t.Fatalf("update must keep subagent_task while sub-agent exists: %#v", stored.Tools)
		}
		if stored.Tools["bash"] != true {
			t.Fatalf("update must keep baseline bash: %#v", stored.Tools)
		}
	}
	afterModel := runUpdate(strings.ReplaceAll(idReplace(abPayloadUpdateModel), "MODEL_ID_FROM_ENUM", "test-model-alt"))
	if afterModel["agent"].(map[string]any)["model"] != "test-model-alt" {
		t.Fatalf("documented model-change payload did not apply: %#v", afterModel["agent"])
	}

	// 9. The errors documented in the skill's recovery table, verbatim fragments.
	createErr := func(args createAgentArgs, wants ...string) {
		t.Helper()
		res, _ := handleCreateAgent(ctx, deps, token, team.ID, frontend, args)
		assertBuilderToolError(t, res, wants...)
	}
	createErr(createAgentArgs{}, "name is required")
	createErr(createAgentArgs{Name: "Bad Tool Agent", Tools: []string{"browser"}}, "unknown tool", "allowed tools are:")
	createErr(createAgentArgs{Name: "Bad Model Agent", Model: "gpt-99"}, "unknown model", "allowed models are:")
	createErr(createAgentArgs{Name: "Bad Skill Agent", Skills: []string{"nope-skill"}}, "unknown skill")
	createErr(createAgentArgs{Name: "Sub Missing", SubAgents: []subAgentToolArgs{{Name: " "}}}, "sub-agent name is required")
	createErr(createAgentArgs{Name: "Sub Dup", SubAgents: []subAgentToolArgs{{Name: "Twin"}, {Name: "Twin"}}}, "duplicate sub-agent name")

	badGet, _ := handleGetAgent(ctx, db, token, frontend, "", getAgentArgs{AgentID: "not-a-uuid"})
	assertBuilderToolError(t, badGet, "agent_id must be a valid UUID")

	strPtr := func(s string) *string { return &s }
	notFound, _ := handleUpdateAgent(ctx, deps, token, frontend, updateAgentArgs{AgentID: uuid.NewString(), Description: strPtr("x")})
	assertBuilderToolError(t, notFound, "agent not found")
	badStatus, _ := handleUpdateAgent(ctx, deps, token, frontend, updateAgentArgs{AgentID: agentID, Status: strPtr("paused")})
	assertBuilderToolError(t, badStatus, "status must be active or archived")

	defaultAgent, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Default Assistant " + uuid.NewString()[:8], TeamID: team.ID})
	if err != nil {
		t.Fatalf("create default agent: %v", err)
	}
	if err := db.Model(&model.Agent{}).Where("id = ?", defaultAgent.ID).Update("is_default", true).Error; err != nil {
		t.Fatalf("mark default: %v", err)
	}
	renameDefault, _ := handleUpdateAgent(ctx, deps, token, frontend, updateAgentArgs{AgentID: defaultAgent.ID.String(), Name: strPtr("New Name")})
	assertBuilderToolError(t, renameDefault, "default agent cannot be renamed")

	// 10. Archive with the documented payload; the agent leaves list_agents.
	runUpdate(idReplace(abPayloadUpdateArchive))
	listRes, _ := handleListAgents(ctx, db, token, "")
	if strings.Contains(errResultText(listRes), agentID) {
		t.Fatalf("archived agent still listed by list_agents")
	}
}
