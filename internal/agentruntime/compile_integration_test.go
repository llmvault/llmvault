package agentruntime

import (
	"context"
	"encoding/json"
	"reflect"
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
	if !strings.Contains(requirePromptString(t, base.Content), "Default to action.") {
		t.Fatalf("base system prompt missing from first cacheable segment: %#v", cacheable[0])
	}
	if !strings.Contains(requirePromptString(t, base.Content), "You are Aria, an AI agent running in Hivy's sandbox environment.") {
		t.Fatalf("base identity missing agent sentence: %q", requirePromptString(t, base.Content))
	}
	instructionsSegment := requireStaticPromptSegment(t, cacheable[1])
	if requirePromptString(t, instructionsSegment.Title) != "Instructions" {
		t.Fatalf("instructions title = %q", requirePromptString(t, instructionsSegment.Title))
	}
	if !strings.Contains(requirePromptString(t, instructionsSegment.Content), "<instructions>\n"+instructions+"\n</instructions>") {
		t.Fatalf("instructions content = %q", requirePromptString(t, instructionsSegment.Content))
	}
	company := requireStaticPromptSegment(t, cacheable[2])
	if requirePromptString(t, company.Title) != "About the company" {
		t.Fatalf("company title = %q", requirePromptString(t, company.Title))
	}
	if !strings.Contains(requirePromptString(t, company.Content), "<company>") {
		t.Fatalf("company content is not XML wrapped: %q", requirePromptString(t, company.Content))
	}
	if len(dynamic) != 3 {
		t.Fatalf("dynamic segment count = %d", len(dynamic))
	}
	if got := requireDynamicContextSegmentType(t, dynamic[0]); got != "dynamic_context" {
		t.Fatalf("first dynamic segment type = %q", got)
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
	if len(dynamic) != 3 {
		t.Fatalf("dynamic_segments count = %d", len(dynamic))
	}
	firstDynamic, _ := dynamic[0].(map[string]any)
	if firstDynamic["type"] != "dynamic_context" {
		t.Fatalf("first dynamic segment type = %#v", firstDynamic["type"])
	}
}

func TestCompile_SerializesSkillOptionalArraysAsEmptyArrays(t *testing.T) {
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
	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if strings.Contains(string(body), `"related_skills":null`) {
		t.Fatalf("related_skills serialized as null: %s", string(body))
	}
	if !strings.Contains(string(body), `"related_skills":[]`) {
		t.Fatalf("related_skills did not serialize as empty array: %s", string(body))
	}
	if strings.Contains(string(body), `"required_environment_variables":null`) {
		t.Fatalf("required_environment_variables serialized as null: %s", string(body))
	}
	if strings.Contains(string(body), `"required_credential_files":null`) {
		t.Fatalf("required_credential_files serialized as null: %s", string(body))
	}
}

func TestCompile_PreservesSkillRequiredEnvironmentVariables(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Skill env-" + uuid.NewString()}
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
		Bundle: model.RawJSON(`{
			"description":"Upload generated artifacts.",
			"content":"Use the upload endpoint.",
			"files":{},
			"required_environment_variables":["HIVY_DRIVE_UPLOAD_BEARER","HIVY_DRIVE_UPLOAD_URL","HIVY_DRIVE_UPLOAD_BEARER"]
		}`),
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
	if len(def.Skills) != 1 {
		t.Fatalf("skills = %#v", def.Skills)
	}
	want := []string{AgentEnvDriveUploadBearer, AgentEnvDriveUploadURL}
	if !reflect.DeepEqual(def.Skills[0].RequiredEnvironmentVariables, want) {
		t.Fatalf("required env vars = %#v, want %#v", def.Skills[0].RequiredEnvironmentVariables, want)
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
