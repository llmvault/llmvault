package agentruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompile_IgnoresArchivedAttachedSkills(t *testing.T) {
	db := connectCompileTestDB(t)
	org := model.Org{Name: "Archived skills-" + uuid.NewString()}
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
	plugin := model.Plugin{ID: uuid.New(), Slug: "drive-" + uuid.NewString(), Name: "Drive", Status: model.PluginStatusActive}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	active := compileTestSkill("drive-"+uuid.NewString(), "Drive", nil)
	archived := compileTestSkill("asset-uploads-"+uuid.NewString(), "asset-uploads", nil)
	active.PluginID = &plugin.ID
	archived.PluginID = &plugin.ID
	archived.Status = model.SkillStatusArchived
	for _, skill := range []model.Skill{active, archived} {
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create skill %s: %v", skill.Slug, err)
		}
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: org.ID, AgentID: agent.ID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin: %v", err)
	}

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.Skills) != 1 {
		t.Fatalf("skills = %#v, want only published skill", def.Skills)
	}
	if def.Skills[0].Name != active.Slug {
		t.Fatalf("compiled skill = %q, want %q", def.Skills[0].Name, active.Slug)
	}
}
