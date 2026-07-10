package pluginresolve

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func containsID(ids []uuid.UUID, want uuid.UUID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestEffectivePluginIDs_AutoInstallIncludedForEveryone(t *testing.T) {
	f := newResolveFixture(t)
	auto := f.seedPlugin(t, true, "resolve-auto", `{"auto_install":true}`, model.PluginStatusActive)

	regular := f.seedAgent(t, f.team.ID, false, nil)
	def := f.seedAgent(t, f.team.ID, true, nil)

	for _, agent := range []model.Agent{regular, def} {
		ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !containsID(ids, auto.ID) {
			t.Fatalf("auto-install plugin missing for agent default=%v", agent.IsDefault)
		}
	}
}

func TestEffectivePluginIDs_DefaultAgentPluginOnlyForDefault(t *testing.T) {
	f := newResolveFixture(t)
	da := f.seedPlugin(t, true, "resolve-default-agent", `{"default_agent_install":true}`, model.PluginStatusActive)

	regular := f.seedAgent(t, f.team.ID, false, nil)
	def := f.seedAgent(t, f.team.ID, true, nil)

	regularIDs, err := EffectivePluginIDs(context.Background(), f.db, regular)
	if err != nil {
		t.Fatalf("resolve regular: %v", err)
	}
	if containsID(regularIDs, da.ID) {
		t.Fatalf("default-agent plugin should not be on a non-default agent")
	}

	defIDs, err := EffectivePluginIDs(context.Background(), f.db, def)
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if !containsID(defIDs, da.ID) {
		t.Fatalf("default-agent plugin missing on the default agent")
	}
}

func TestEffectivePluginIDs_TeamGrantIncludedAndScoped(t *testing.T) {
	f := newResolveFixture(t)
	plugin := f.seedPlugin(t, false, "resolve-team", `{}`, model.PluginStatusActive)
	f.installOrgPlugin(t, plugin.ID, false)
	f.grantTeamPlugin(t, f.team.ID, plugin.ID)

	inTeam := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, inTeam)
	if err != nil {
		t.Fatalf("resolve in-team: %v", err)
	}
	if !containsID(ids, plugin.ID) {
		t.Fatalf("team-granted plugin missing on a team agent")
	}

	otherTeam := model.Team{ID: uuid.New(), OrgID: f.org.ID, Name: "resolve-other-" + uuid.NewString()[:8]}
	if err := f.db.Create(&otherTeam).Error; err != nil {
		t.Fatalf("create other team: %v", err)
	}
	t.Cleanup(func() { f.db.Where("id = ?", otherTeam.ID).Delete(&model.Team{}) })
	outAgent := f.seedAgent(t, otherTeam.ID, false, nil)
	outIDs, err := EffectivePluginIDs(context.Background(), f.db, outAgent)
	if err != nil {
		t.Fatalf("resolve out-team: %v", err)
	}
	if containsID(outIDs, plugin.ID) {
		t.Fatalf("team-granted plugin leaked to an agent in another team")
	}
}

func TestEffectivePluginIDs_AgentOverrideDisablesOnlyThatTeamPlugin(t *testing.T) {
	f := newResolveFixture(t)
	teamPlugin := f.seedPlugin(t, false, "resolve-team-override", `{}`, model.PluginStatusActive)
	autoPlugin := f.seedPlugin(t, true, "resolve-auto-override", `{"auto_install":true}`, model.PluginStatusActive)
	f.installOrgPlugin(t, teamPlugin.ID, false)
	f.grantTeamPlugin(t, f.team.ID, teamPlugin.ID)

	disabled := f.seedAgent(t, f.team.ID, false, nil)
	enabled := f.seedAgent(t, f.team.ID, false, nil)
	f.disableAgentPlugin(t, disabled.ID, teamPlugin.ID)

	disabledIDs, err := EffectivePluginIDs(context.Background(), f.db, disabled)
	if err != nil {
		t.Fatalf("resolve disabled agent: %v", err)
	}
	if containsID(disabledIDs, teamPlugin.ID) {
		t.Fatal("disabled team plugin remained effective")
	}
	if !containsID(disabledIDs, autoPlugin.ID) {
		t.Fatal("agent override must not remove auto-install plugin")
	}

	enabledIDs, err := EffectivePluginIDs(context.Background(), f.db, enabled)
	if err != nil {
		t.Fatalf("resolve enabled agent: %v", err)
	}
	if !containsID(enabledIDs, teamPlugin.ID) {
		t.Fatal("agent override leaked to another agent")
	}
}

func TestEffectivePluginIDs_OrgUninstalledOrRevokedExcluded(t *testing.T) {
	f := newResolveFixture(t)

	notInstalled := f.seedPlugin(t, false, "resolve-noinstall", `{}`, model.PluginStatusActive)
	f.grantTeamPlugin(t, f.team.ID, notInstalled.ID)

	revoked := f.seedPlugin(t, false, "resolve-revoked", `{}`, model.PluginStatusActive)
	f.installOrgPlugin(t, revoked.ID, true)
	f.grantTeamPlugin(t, f.team.ID, revoked.ID)

	agent := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if containsID(ids, notInstalled.ID) {
		t.Fatalf("team grant without an org install must be excluded")
	}
	if containsID(ids, revoked.ID) {
		t.Fatalf("team grant with a revoked org install must be excluded")
	}
}

func TestEffectivePluginIDs_InactivePluginExcluded(t *testing.T) {
	f := newResolveFixture(t)
	inactive := f.seedPlugin(t, false, "resolve-inactive", `{}`, model.PluginStatusArchived)
	f.installOrgPlugin(t, inactive.ID, false)
	f.grantTeamPlugin(t, f.team.ID, inactive.ID)

	agent := f.seedAgent(t, f.team.ID, false, nil)
	ids, err := EffectivePluginIDs(context.Background(), f.db, agent)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if containsID(ids, inactive.ID) {
		t.Fatalf("inactive plugin must be excluded from the effective set")
	}
}
