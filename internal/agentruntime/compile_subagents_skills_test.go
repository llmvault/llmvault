package agentruntime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompile_SubAgentsInheritParentSkillsAndCanLoadThem(t *testing.T) {
	db := connectCompileTestDB(t)
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
		Slug:   "deck-" + uuid.NewString(),
		Name:   "Deck",
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	skill := model.Skill{
		ID:         uuid.New(),
		OrgID:      &org.ID,
		PluginID:   &plugin.ID,
		Slug:       "deck-review",
		Name:       "Deck Review",
		SourceType: model.SkillSourceInline,
		RepoRef:    "main",
		Status:     model.SkillStatusPublished,
		Bundle: model.RawJSON(
			`{"description":"Review decks.","content":"Review the deck carefully.","files":{}}`,
		),
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: agent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}
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
