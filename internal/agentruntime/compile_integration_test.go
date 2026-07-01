package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompile_EmitsControlPlaneSystemPromptWithoutRawAgentPrompt(t *testing.T) {
	db := connectCompileTestDB(t)
	orgName := "Acme-" + uuid.NewString()
	org := model.Org{
		Name:          orgName,
		PromptCompany: "Acme builds deployment tools for engineering teams.",
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	description := "Coordinates platform engineering work."
	instructions := "Use production telemetry before recommending a rollback."
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		Name:          "Aria",
		Description:   &description,
		Instructions:  &instructions,
		Model:         DefaultAgentModel,
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if !strings.Contains(string(body), `"system_prompt"`) {
		t.Fatalf("agent config must include system_prompt: %s", string(body))
	}
	if strings.Contains(string(body), `"prompt_fragments"`) {
		t.Fatalf("agent config included prompt_fragments: %s", string(body))
	}
	assertRuntimeSystemPromptPayloadShape(t, body)
	cacheable := requireCacheableSegments(t, def.SystemPrompt)
	dynamic := requireDynamicSegments(t, def.SystemPrompt)
	if len(cacheable) < 3 {
		t.Fatalf("expected base, instructions, and company prompt segments: %#v", cacheable)
	}
	base := requireStaticPromptSegment(t, cacheable[0])
	baseContent := requirePromptString(t, base.Content)
	if !strings.Contains(baseContent, "Default to action.") {
		t.Fatalf("base system prompt missing from first cacheable segment: %#v", cacheable[0])
	}
	if !strings.Contains(baseContent, "You are Aria, an AI agent running in Hivy's sandbox environment.") {
		t.Fatalf("base identity missing agent sentence: %q", baseContent)
	}
	if strings.Contains(baseContent, "model_adapter") {
		t.Fatalf("base must not carry a standalone model_adapter section: %q", baseContent)
	}
	if !strings.Contains(baseContent, "- Prefer concrete tool calls over extended thinking.") {
		t.Fatalf("model adapter guidance did not fold into the base tool contract: %q", baseContent)
	}
	instructionsSegment := requireStaticPromptSegmentByTitle(t, cacheable, "Instructions")
	if !strings.Contains(requirePromptString(t, instructionsSegment.Content), "<instructions>\n"+instructions+"\n</instructions>") {
		t.Fatalf("instructions content = %q", requirePromptString(t, instructionsSegment.Content))
	}
	company := requireStaticPromptSegmentByTitle(t, cacheable, "About the company")
	if !strings.Contains(requirePromptString(t, company.Content), "<company>") {
		t.Fatalf("company content is not XML wrapped: %q", requirePromptString(t, company.Content))
	}
	if len(dynamic) != 1 {
		t.Fatalf("dynamic segment count = %d (want 1: only mcp_tools when agent has no skills)", len(dynamic))
	}
	if got := requireListSegment3Type(t, dynamic[0]); got != "mcp_tools" {
		t.Fatalf("first dynamic segment type = %q, want mcp_tools", got)
	}
}

func assertRuntimeSystemPromptPayloadShape(t *testing.T, body []byte) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode compiled config JSON: %v", err)
	}
	agent, _ := doc["agent"].(map[string]any)
	if _, ok := agent["system_prompt"]; ok {
		t.Fatalf("agent metadata must not contain raw system_prompt: %s", string(body))
	}
	systemPrompt, ok := doc["system_prompt"].(map[string]any)
	if !ok {
		t.Fatalf("system_prompt missing or wrong shape: %s", string(body))
	}
	cacheable, _ := systemPrompt["cacheable_segments"].([]any)
	if len(cacheable) == 0 {
		t.Fatalf("cacheable_segments missing: %s", string(body))
	}
	firstCacheable, _ := cacheable[0].(map[string]any)
	if firstCacheable["type"] != "static_text" {
		t.Fatalf("first cacheable segment type = %#v", firstCacheable["type"])
	}
	dynamic, _ := systemPrompt["dynamic_segments"].([]any)
	if len(dynamic) < 1 {
		t.Fatalf("dynamic_segments missing: %s", string(body))
	}
	// The last dynamic segment is always mcp_tools; there is no skill_catalog
	// segment (skills are now served as MCP tools).
	lastDynamic, _ := dynamic[len(dynamic)-1].(map[string]any)
	if lastDynamic["type"] != "mcp_tools" {
		t.Fatalf("last dynamic segment type = %#v, want mcp_tools", lastDynamic["type"])
	}
}

func TestCompile_SkillsAppearsAsStaticPromptHintWhenInstalled(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Skills-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	category := "engineering"
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		Name:          "Aria",
		Category:      &category,
		Model:         DefaultAgentModel,
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		Integrations:  model.JSON{},
		Resources:     model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	desc := "Upload generated artifacts."
	plugin := model.Plugin{ID: uuid.New(), Slug: "drive-" + uuid.NewString(), Name: "Drive", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	skill := model.Skill{
		ID:          uuid.New(),
		OrgID:       &org.ID,
		PluginID:    &plugin.ID,
		Slug:        "drive",
		Name:        "Drive",
		Description: &desc,
		SourceType:  model.SkillSourceInline,
		RepoRef:     "main",
		Status:      model.SkillStatusPublished,
		Bundle:      model.RawJSON(`{"description":"Upload generated artifacts.","content":"Use the upload endpoint.","files":{}}`),
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: agent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// When the agent has installed skills the first dynamic segment must be a
	// static "Available skills" hint (prepended by renderSkillHint).
	dynamic := requireDynamicSegments(t, def.SystemPrompt)
	if len(dynamic) < 2 {
		t.Fatalf("expected skill hint + mcp_tools dynamic segments, got %d", len(dynamic))
	}
	skillsSegment := requireStaticPromptSegment(t, dynamic[0])
	if skillsSegment.Title == nil || *skillsSegment.Title != "Available skills" {
		t.Fatalf("first dynamic segment title = %v, want Available skills", skillsSegment.Title)
	}
	if got := requireListSegment3Type(t, dynamic[len(dynamic)-1]); got != "mcp_tools" {
		t.Fatalf("last dynamic segment = %q, want mcp_tools", got)
	}
}


func compileTestSkill(slug, name string, orgID *uuid.UUID) model.Skill {
	desc := name + " description"
	return model.Skill{
		ID:          uuid.New(),
		OrgID:       orgID,
		Slug:        slug,
		Name:        name,
		Description: &desc,
		SourceType:  model.SkillSourceInline,
		RepoRef:     "main",
		Status:      model.SkillStatusPublished,
		Bundle:      model.RawJSON(`{"description":"Test skill.","content":"Follow this test skill.","files":{}}`),
	}
}
