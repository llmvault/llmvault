package agentruntime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompile_SubAgentsInheritParentSkillsAndCanLoadThem(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createAgentWithPluginSkills(t, db, []skillSeed{{Slug: "deck-review", Name: "Deck Review", Description: "Review decks."}})
	rawSubAgents, err := json.Marshal(map[string]model.AgentCatalogSubAgent{
		"reviewer": {
			Name:         "Reviewer",
			Description:  "Reviews delegated work.",
			Tools:        model.JSON{"read_file": true, "skills_list": false, "skill_view": false},
			Instructions: "Use the right skill before reviewing.",
		},
	})
	if err != nil {
		t.Fatalf("marshal subagents: %v", err)
	}
	agent.AgentCatalog = &model.AgentCatalog{SubAgents: model.RawJSON(rawSubAgents)}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.Skills) != 1 || def.Skills[0].Name != "deck-review" {
		t.Fatalf("parent skills = %#v", def.Skills)
	}
	subAgent := def.SubAgents["reviewer"]
	if subAgent == nil {
		t.Fatalf("missing reviewer subagent: %#v", def.SubAgents)
	}
	if len(subAgent.Skills) != 1 || subAgent.Skills[0].Name != "deck-review" {
		t.Fatalf("subagent skills = %#v, want inherited parent skill", subAgent.Skills)
	}
	wantTools := []string{"builtin.read_file", "builtin.skills_list", "builtin.skill_view"}
	if got := runtimeToolTypes(subAgent.Tools); !reflect.DeepEqual(got, wantTools) {
		t.Fatalf("subagent tools = %#v, want %#v", got, wantTools)
	}
}

func TestCompile_SkillFiltersDoNotTrimRuntimeSkillBundles(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createAgentWithPluginSkills(t, db, []skillSeed{
		{Slug: "asset-export", Name: "Asset Export", Description: "Prepare assets."},
		{Slug: "brand-system", Name: "Brand System", Description: "Extract brand systems."},
		{Slug: "canvas-sync", Name: "Canvas Sync", Description: "Sync canvas work."},
		{Slug: "deck-review", Name: "Deck Review", Description: "Review decks."},
	})
	agent.Skills = model.JSON{
		"skill_filter": model.JSON{
			"allow": []any{"deck-review", "brand-system", "deck-review", " "},
		},
	}
	rawSubAgents, err := json.Marshal(map[string]model.AgentCatalogSubAgent{
		"design-worker": {
			Name:         "Design Worker",
			Description:  "Creates concrete design artifacts.",
			Tools:        model.JSON{"read_file": true},
			SkillFilter:  &model.SkillFilter{Allow: []string{"canvas-sync", "asset-export", "canvas-sync", " "}},
			Instructions: "Use implementation design skills only.",
		},
		"no-skills": {
			Name:         "No Skills",
			Description:  "Runs without visible skills.",
			Tools:        model.JSON{"read_file": true},
			SkillFilter:  &model.SkillFilter{Allow: []string{}},
			Instructions: "Do not load skills.",
		},
	})
	if err != nil {
		t.Fatalf("marshal subagents: %v", err)
	}
	agent.AgentCatalog = &model.AgentCatalog{SubAgents: model.RawJSON(rawSubAgents)}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	wantAllSkills := []string{"asset-export", "brand-system", "canvas-sync", "deck-review"}
	if got := skillNames(def.Skills); !reflect.DeepEqual(got, wantAllSkills) {
		t.Fatalf("parent skills = %#v, want full runtime bundle %#v", got, wantAllSkills)
	}
	if def.SkillFilter == nil || !reflect.DeepEqual(def.SkillFilter.Allow, []string{"brand-system", "deck-review"}) {
		t.Fatalf("parent skill filter = %#v", def.SkillFilter)
	}
	subAgent := def.SubAgents["design-worker"]
	if subAgent == nil {
		t.Fatalf("missing design-worker subagent: %#v", def.SubAgents)
	}
	if got := skillNames(subAgent.Skills); !reflect.DeepEqual(got, wantAllSkills) {
		t.Fatalf("subagent skills = %#v, want full runtime bundle %#v", got, wantAllSkills)
	}
	if subAgent.SkillFilter == nil || !reflect.DeepEqual(subAgent.SkillFilter.Allow, []string{"asset-export", "canvas-sync"}) {
		t.Fatalf("subagent skill filter = %#v", subAgent.SkillFilter)
	}
	noSkills := def.SubAgents["no-skills"]
	if noSkills == nil {
		t.Fatalf("missing no-skills subagent: %#v", def.SubAgents)
	}
	if got := skillNames(noSkills.Skills); !reflect.DeepEqual(got, wantAllSkills) {
		t.Fatalf("no-skills subagent skills = %#v, want full runtime bundle %#v", got, wantAllSkills)
	}
	if noSkills.SkillFilter == nil || len(noSkills.SkillFilter.Allow) != 0 {
		t.Fatalf("empty subagent skill filter = %#v, want explicit empty allow list", noSkills.SkillFilter)
	}
}

func TestCompile_LoadsCatalogSkillFiltersWithoutDroppingRuntimeBundles(t *testing.T) {
	db := connectCompileTestDB(t)
	agent := createAgentWithPluginSkills(t, db, []skillSeed{
		{Slug: "deck-review", Name: "Deck Review", Description: "Review decks."},
		{Slug: "research-notes", Name: "Research Notes", Description: "Research context."},
	})
	rawSubAgents, err := json.Marshal(map[string]model.AgentCatalogSubAgent{
		"researcher": {
			Name:          "Researcher",
			Description:   "Researches delegated work.",
			Tools:         model.JSON{"read_file": true},
			McpToolFilter: &model.ToolFilter{Allow: []string{"generate_vector_image", "generate_image", "generate_image"}},
			SkillFilter:   &model.SkillFilter{Allow: []string{"research-notes"}},
			Instructions:  "Use research skills only.",
		},
	})
	if err != nil {
		t.Fatalf("marshal subagents: %v", err)
	}
	catalog := model.AgentCatalog{
		ID:        uuid.New(),
		Slug:      "catalog-skill-filter-" + uuid.NewString(),
		Name:      "Catalog Skill Filter",
		SubAgents: model.RawJSON(rawSubAgents),
		Manifest:  model.RawJSON(`{"mcp_tool_filter":{"deny":["generate_image","generate_vector_image"]},"skill_filter":{"allow":["deck-review"]}}`),
		Status:    model.AgentCatalogStatusActive,
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	agent.AgentCatalogID = &catalog.ID

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	wantAllSkills := []string{"deck-review", "research-notes"}
	if got := skillNames(def.Skills); !reflect.DeepEqual(got, wantAllSkills) {
		t.Fatalf("parent skills = %#v, want full runtime bundle %#v", got, wantAllSkills)
	}
	if def.SkillFilter == nil || !reflect.DeepEqual(def.SkillFilter.Allow, []string{"deck-review"}) {
		t.Fatalf("parent skill filter = %#v", def.SkillFilter)
	}
	if def.McpToolFilter == nil || !reflect.DeepEqual(def.McpToolFilter.Deny, []string{"generate_image", "generate_vector_image"}) || def.McpToolFilter.Allow != nil {
		t.Fatalf("parent mcp tool filter = %#v", def.McpToolFilter)
	}
	subAgent := def.SubAgents["researcher"]
	if subAgent == nil {
		t.Fatalf("missing researcher subagent: %#v", def.SubAgents)
	}
	if got := skillNames(subAgent.Skills); !reflect.DeepEqual(got, wantAllSkills) {
		t.Fatalf("subagent skills = %#v, want full runtime bundle %#v", got, wantAllSkills)
	}
	if subAgent.SkillFilter == nil || !reflect.DeepEqual(subAgent.SkillFilter.Allow, []string{"research-notes"}) {
		t.Fatalf("subagent skill filter = %#v", subAgent.SkillFilter)
	}
	if subAgent.McpToolFilter == nil || !reflect.DeepEqual(subAgent.McpToolFilter.Allow, []string{"generate_image", "generate_vector_image"}) || subAgent.McpToolFilter.Deny != nil {
		t.Fatalf("subagent mcp tool filter = %#v", subAgent.McpToolFilter)
	}
}

type skillSeed struct {
	Slug        string
	Name        string
	Description string
}

func createAgentWithPluginSkills(t *testing.T, db *gorm.DB, seeds []skillSeed) model.Agent {
	t.Helper()
	org := model.Org{Name: "Subagent skills-" + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		Name:          "Aria",
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
	plugin := model.Plugin{
		ID:     uuid.New(),
		Slug:   "skills-" + uuid.NewString(),
		Name:   "Skills",
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	for _, seed := range seeds {
		skill := model.Skill{
			ID:         uuid.New(),
			OrgID:      &org.ID,
			PluginID:   &plugin.ID,
			Slug:       seed.Slug,
			Name:       seed.Name,
			SourceType: model.SkillSourceInline,
			RepoRef:    "main",
			Status:     model.SkillStatusPublished,
			Bundle: model.RawJSON(
				`{"description":"` + seed.Description + `","content":"Use this skill carefully.","files":{}}`,
			),
		}
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create skill %s: %v", seed.Slug, err)
		}
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: agent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
	return agent
}

func skillNames(skills []SkillSpec) []string {
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skill.Name)
	}
	return out
}
