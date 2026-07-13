package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestListTeamPluginsAndSkillValidationStayTeamScoped(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	teamA := testTeam(t, db, org.ID)
	teamB := testTeam(t, db, org.ID)

	caller := model.Agent{
		ID:            uuid.New(),
		OrgID:         &org.ID,
		TeamID:        teamA.ID,
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
	if err := db.Create(&caller).Error; err != nil {
		t.Fatalf("create calling agent: %v", err)
	}

	createTeamPlugin := func(team model.Team, name string) model.Plugin {
		t.Helper()
		plugin := model.Plugin{
			ID:       uuid.New(),
			OrgID:    &org.ID,
			TeamID:   &team.ID,
			Slug:     "shared-toolkit",
			Name:     name,
			Status:   model.PluginStatusActive,
			Manifest: model.RawJSON(`{"team_plugin":true}`),
		}
		if err := db.Create(&plugin).Error; err != nil {
			t.Fatalf("create team plugin: %v", err)
		}
		if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID}).Error; err != nil {
			t.Fatalf("install team plugin: %v", err)
		}
		grantTeamPlugin(t, db, org.ID, team.ID, plugin.ID)
		return plugin
	}

	pluginA := createTeamPlugin(teamA, "Team A Toolkit")
	pluginB := createTeamPlugin(teamB, "Team B Toolkit")
	desc := "Use when working with the owning team's process."
	for _, fixture := range []struct {
		plugin model.Plugin
		slug   string
	}{
		{plugin: pluginA, slug: "team-a-process"},
		{plugin: pluginB, slug: "team-b-process"},
	} {
		pluginID := fixture.plugin.ID
		skill := model.Skill{
			ID:          uuid.New(),
			OrgID:       &org.ID,
			PluginID:    &pluginID,
			Slug:        fixture.slug,
			Name:        fixture.slug,
			Description: &desc,
			SourceType:  model.SkillSourceInline,
			Status:      model.SkillStatusPublished,
			Bundle:      model.RawJSON(`{}`),
		}
		if err := db.Create(&skill).Error; err != nil {
			t.Fatalf("create team skill: %v", err)
		}
	}

	token := &model.Token{OrgID: org.ID, Meta: model.JSON{
		model.TokenMetaType:    model.TokenTypeAgentProxy,
		model.TokenMetaAgentID: caller.ID.String(),
	}}
	result, err := handleListTeamPlugins(context.Background(), db, token, "https://app.test")
	if err != nil {
		t.Fatalf("list team plugins: %v", err)
	}
	payload := builderResultJSON(t, result)
	body, _ := json.Marshal(payload)
	text := string(body)
	if !strings.Contains(text, pluginA.Name) || !strings.Contains(text, "team-a-process") {
		t.Fatalf("calling team's plugin missing: %s", text)
	}
	if strings.Contains(text, pluginB.Name) || strings.Contains(text, "team-b-process") {
		t.Fatalf("other team's plugin leaked: %s", text)
	}

	if _, err := validateSkillSlugs(context.Background(), db, org.ID, teamA.ID, []string{"team-b-process"}); err == nil {
		t.Fatal("another team's skill must not validate")
	}
	if got, err := validateSkillSlugs(context.Background(), db, org.ID, teamB.ID, []string{"team-b-process"}); err != nil || len(got) != 1 {
		t.Fatalf("owning team skill rejected: got=%v err=%v", got, err)
	}
}
