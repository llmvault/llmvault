package agents

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/usehivy/hivy/internal/model"
)

// The payloads below are copied VERBATIM from the shipped skill at
// global/plugins/agent-builder/skills/agent-builder/SKILL.md. The skill is the
// payload contract: if a schema change breaks this test, the SKILL.md must
// change in the same commit (TestAgentBuilderSkillPayloadContract verifies the
// constants still appear verbatim in the file). Placeholder values
// ("7c9e6679-…", "github", "github-triage", "MODEL_ID_FROM_ENUM") are swapped
// for real ones at runtime; the JSON shape is untouched.
const (
	abPayloadListEmpty = `{}`
	abPayloadGetAgent  = `{ "agent_id": "7c9e6679-…" }`
	abPayloadCreate    = `{
  "name": "Support Triage",
  "description": "Triages incoming support requests, drafts replies, and escalates edge cases to a human.",
  "instructions": "You are Support Triage for the team's support inbox.\n\nYour job: classify each incoming request (bug, billing, how-to, feature request), answer the ones covered by known solutions, and escalate anything ambiguous, angry, or contractual to a human — never guess on those.\n\nHow to work: read the full request first. Search the web only when the answer likely changed recently. When you draft a reply, delegate to your Responder sub-agent and review its draft before sending.\n\nBoundaries: never promise refunds, legal terms, or timelines. Never reply to legal threats — escalate.\n\nVoice: warm, direct, under 8 sentences.",
  "plugin_slugs": ["github"],
  "skills": ["github-triage"],
  "tools": ["web_search", "web_fetch", "skills_list", "skill_view"],
  "sub_agents": [
    {
      "name": "Responder",
      "description": "Drafts the customer-facing reply for requests Triage has classified.",
      "instructions": "Draft a reply for the classified support request you are given. Match the team voice: warm, direct, no filler. Return only the draft.",
      "tools": ["web_fetch"]
    }
  ]
}`
	abPayloadUpdateDescription = `{ "agent_id": "7c9e6679-…", "description": "Triages support requests for the EU team." }`
	abPayloadUpdateArchive     = `{ "agent_id": "7c9e6679-…", "status": "archived" }`
	abPayloadUpdateAddTool     = `{
  "agent_id": "7c9e6679-…",
  "tools": ["web_search", "web_fetch", "skills_list", "skill_view", "subagent_task", "generate_image"]
}`
	abPayloadUpdateModel = `{ "agent_id": "7c9e6679-…", "model": "MODEL_ID_FROM_ENUM" }`
)

// assertBuilderStrictDecode pins skill↔schema key consistency: every key in the
// documented payload (including nested sub_agents objects) must exist on the
// tool's argument struct.
func assertBuilderStrictDecode(t *testing.T, payload string, dst any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("SKILL.md payload does not decode into the tool args (schema drift): %v\n%s", err, payload)
	}
}

func builderResultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatalf("nil tool result")
	}
	if res.IsError {
		t.Fatalf("tool returned error: %s", errResultText(res))
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(errResultText(res)), &out); err != nil {
		t.Fatalf("tool result is not JSON: %v\n%s", err, errResultText(res))
	}
	return out
}

func assertBuilderToolError(t *testing.T, res *mcp.CallToolResult, wants ...string) {
	t.Helper()
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError result containing %q, got: %#v", wants, res)
	}
	text := errResultText(res)
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("error %q does not contain %q", text, want)
		}
	}
}

func agentToolStrings(t *testing.T, agentObj map[string]any, key string) []string {
	t.Helper()
	raw, ok := agentObj[key].([]any)
	if !ok {
		t.Fatalf("agent object has no %q array: %#v", key, agentObj)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

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
	skillSlug := "github-triage-" + uuid.NewString()[:8]
	plugin := seedInstalledPlugin(t, db, org.ID, "github", skillSlug)
	deps := Deps{DB: db, DefaultModel: "deepseek-v4-flash", Models: []string{"deepseek-v4-flash", "test-model-alt"}}
	token := &model.Token{OrgID: org.ID}
	const frontend = "https://app.test"

	replace := strings.NewReplacer(
		"github-triage", skillSlug,
		"github", plugin.Slug,
	).Replace

	// 3. The documented empty payloads for the two list tools.
	if res, _ := handleListAgents(ctx, db, token); res == nil || res.IsError {
		t.Fatalf("list_agents with documented %s payload failed", abPayloadListEmpty)
	}
	pluginsOut := map[string]any{}
	{
		res, _ := handleListOrgPlugins(ctx, db, token, frontend)
		pluginsOut = builderResultJSON(t, res)
	}
	installedText, _ := json.Marshal(pluginsOut["installed"])
	for _, want := range []string{plugin.Slug, skillSlug, "install_url"} {
		if !strings.Contains(string(installedText), want) {
			t.Fatalf("list_org_plugins installed list missing %q: %s", want, installedText)
		}
	}

	// 4. create_agent with the documented Support Triage payload.
	var cArgs createAgentArgs
	if err := json.Unmarshal([]byte(replace(abPayloadCreate)), &cArgs); err != nil {
		t.Fatalf("create payload: %v", err)
	}
	createRes, _ := handleCreateAgent(ctx, deps, token, frontend, cArgs)
	created := builderResultJSON(t, createRes)
	agentObj := created["agent"].(map[string]any)
	agentID := agentObj["id"].(string)
	if created["url"] == "" {
		t.Fatalf("create_agent response has no url")
	}
	tools := agentToolStrings(t, agentObj, "tools")
	for _, want := range []string{"web_search", "web_fetch", "skills_list", "skill_view", "subagent_task"} {
		if !containsString(tools, want) {
			t.Fatalf("created agent tools missing %q (subagent_task must be auto-added): %v", want, tools)
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
	getRes, _ := handleGetAgent(ctx, db, token, frontend, gArgs)
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
	for _, want := range []string{"web_search", "generate_image", "subagent_task"} {
		if !containsString(addedTools, want) {
			t.Fatalf("add-one-tool update lost %q: %v", want, addedTools)
		}
	}
	afterModel := runUpdate(strings.ReplaceAll(idReplace(abPayloadUpdateModel), "MODEL_ID_FROM_ENUM", "test-model-alt"))
	if afterModel["agent"].(map[string]any)["model"] != "test-model-alt" {
		t.Fatalf("documented model-change payload did not apply: %#v", afterModel["agent"])
	}

	// 9. The errors documented in the skill's recovery table, verbatim fragments.
	notInstalled := model.Plugin{ID: uuid.New(), Slug: "vercel-" + uuid.NewString()[:8], Name: "Vercel Test", Status: model.PluginStatusActive, Manifest: model.RawJSON(`{}`)}
	if err := db.Create(&notInstalled).Error; err != nil {
		t.Fatalf("create uninstalled plugin: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", notInstalled.ID).Delete(&model.Plugin{}) })

	createErr := func(args createAgentArgs, wants ...string) {
		t.Helper()
		res, _ := handleCreateAgent(ctx, deps, token, frontend, args)
		assertBuilderToolError(t, res, wants...)
	}
	createErr(createAgentArgs{}, "name is required")
	createErr(createAgentArgs{Name: "Bad Tool Agent", Tools: []string{"browser"}}, "unknown tool", "allowed tools are:")
	createErr(createAgentArgs{Name: "Bad Model Agent", Model: "gpt-99"}, "unknown model", "allowed models are:")
	createErr(createAgentArgs{Name: "Bad Skill Agent", Skills: []string{"nope-skill"}}, "unknown skill")
	createErr(createAgentArgs{Name: "Bad Plugin Agent", PluginSlugs: []string{"nope-plugin"}}, "unknown plugin")
	createErr(createAgentArgs{Name: "Uninstalled Plugin Agent", PluginSlugs: []string{notInstalled.Slug}}, "is not installed for this org")
	createErr(createAgentArgs{Name: "Support Triage"}, "agent name already exists")
	createErr(createAgentArgs{Name: "Sub Missing", SubAgents: []subAgentToolArgs{{Name: " "}}}, "sub-agent name is required")
	createErr(createAgentArgs{Name: "Sub Dup", SubAgents: []subAgentToolArgs{{Name: "Twin"}, {Name: "Twin"}}}, "duplicate sub-agent name")

	badGet, _ := handleGetAgent(ctx, db, token, frontend, getAgentArgs{AgentID: "not-a-uuid"})
	assertBuilderToolError(t, badGet, "agent_id must be a valid UUID")

	strPtr := func(s string) *string { return &s }
	notFound, _ := handleUpdateAgent(ctx, deps, token, frontend, updateAgentArgs{AgentID: uuid.NewString(), Description: strPtr("x")})
	assertBuilderToolError(t, notFound, "agent not found")
	badStatus, _ := handleUpdateAgent(ctx, deps, token, frontend, updateAgentArgs{AgentID: agentID, Status: strPtr("paused")})
	assertBuilderToolError(t, badStatus, "status must be active or archived")

	defaultAgent, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Default Assistant " + uuid.NewString()[:8]})
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
	listRes, _ := handleListAgents(ctx, db, token)
	if strings.Contains(errResultText(listRes), agentID) {
		t.Fatalf("archived agent still listed by list_agents")
	}
}
