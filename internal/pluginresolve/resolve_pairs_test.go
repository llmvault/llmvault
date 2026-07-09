package pluginresolve

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (f resolveFixture) seedGitHubPair(t *testing.T) (model.Plugin, model.Plugin) {
	t.Helper()
	primary := f.seedPlugin(t, true, "resolve-github", `{}`, model.PluginStatusActive)
	f.addIntegration(t, primary.ID, "github-app")
	f.installOrgPlugin(t, primary.ID, false)
	f.grantTeamPlugin(t, f.team.ID, primary.ID)

	reviews := f.seedPlugin(t, true, "resolve-github-reviews", `{}`, model.PluginStatusActive)
	f.addIntegration(t, reviews.ID, "github-app-code-reviews")
	f.installOrgPlugin(t, reviews.ID, false)
	f.grantTeamPlugin(t, f.team.ID, reviews.ID)
	return primary, reviews
}

func TestEffectivePluginIDs_PairRulePlainAgentKeepsPrimary(t *testing.T) {
	f := newResolveFixture(t)
	primary, reviews := f.seedGitHubPair(t)

	agent := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !containsID(ids, primary.ID) {
		t.Fatalf("plain agent should keep the github-app plugin")
	}
	if containsID(ids, reviews.ID) {
		t.Fatalf("plain agent must not also carry the code-reviews plugin")
	}
}

func TestEffectivePluginIDs_PairRuleCatalogRequiredKeepsReviews(t *testing.T) {
	f := newResolveFixture(t)
	primary, reviews := f.seedGitHubPair(t)
	catalog := f.seedCatalog(t, reviews.Slug)

	agent := f.seedAgent(t, f.team.ID, false, &catalog.ID)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !containsID(ids, reviews.ID) {
		t.Fatalf("catalog-required agent should keep the code-reviews plugin")
	}
	if containsID(ids, primary.ID) {
		t.Fatalf("catalog-required agent must not also carry the github-app plugin")
	}
}

func TestEffectivePluginIDs_SingleGitHubPluginUntouched(t *testing.T) {
	f := newResolveFixture(t)
	primary := f.seedPlugin(t, true, "resolve-github-solo", `{}`, model.PluginStatusActive)
	f.addIntegration(t, primary.ID, "github-app")
	f.installOrgPlugin(t, primary.ID, false)
	f.grantTeamPlugin(t, f.team.ID, primary.ID)

	agent := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !containsID(ids, primary.ID) {
		t.Fatalf("a team's single github plugin must survive the pair rule")
	}
}

func TestEffectiveAgentIDsForPlugin_ReverseMapping(t *testing.T) {
	f := newResolveFixture(t)
	teamPlugin := f.seedPlugin(t, false, "resolve-reverse", `{}`, model.PluginStatusActive)
	f.installOrgPlugin(t, teamPlugin.ID, false)
	f.grantTeamPlugin(t, f.team.ID, teamPlugin.ID)

	inTeam := f.seedAgent(t, f.team.ID, false, nil)

	otherTeam := model.Team{ID: uuid.New(), OrgID: f.org.ID, Name: "resolve-rev-other-" + uuid.NewString()[:8]}
	if err := f.db.Create(&otherTeam).Error; err != nil {
		t.Fatalf("create other team: %v", err)
	}
	t.Cleanup(func() { f.db.Where("id = ?", otherTeam.ID).Delete(&model.Team{}) })
	outAgent := f.seedAgent(t, otherTeam.ID, false, nil)

	agentIDs, err := EffectiveAgentIDsForPlugin(context.Background(), f.db, f.org.ID, teamPlugin.ID)
	if err != nil {
		t.Fatalf("reverse map: %v", err)
	}
	if !containsID(agentIDs, inTeam.ID) {
		t.Fatalf("granting team's agent missing from reverse mapping")
	}
	if containsID(agentIDs, outAgent.ID) {
		t.Fatalf("non-granting team's agent should not appear in reverse mapping")
	}
}
