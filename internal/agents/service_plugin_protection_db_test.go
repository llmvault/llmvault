package agents

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// seedInstalledPluginWithManifest creates an active plugin with the given
// manifest flags, installs it for the org, and returns it. The slug carries a
// random suffix so parallel runs don't collide on the unique index.
func seedInstalledPluginWithManifest(t *testing.T, db *gorm.DB, orgID uuid.UUID, slugPrefix, manifest string) model.Plugin {
	t.Helper()
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     slugPrefix + "-" + uuid.NewString()[:8],
		Name:     "Test " + slugPrefix,
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(manifest),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin %q: %v", slugPrefix, err)
	}
	if err := db.Create(&model.OrgPluginInstall{ID: uuid.New(), OrgID: orgID, PluginID: plugin.ID}).Error; err != nil {
		t.Fatalf("install plugin %q for org: %v", slugPrefix, err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	return plugin
}

func attachAgentPlugin(t *testing.T, db *gorm.DB, orgID, agentID, pluginID uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("attach plugin to agent: %v", err)
	}
}

func agentPluginInstalled(db *gorm.DB, agentID, pluginID uuid.UUID) bool {
	var count int64
	db.Model(&model.AgentPluginInstall{}).
		Where("agent_id = ? AND plugin_id = ?", agentID, pluginID).
		Count(&count)
	return count > 0
}

// UpdateAgent replaces the agent's plugin set with whatever plugin_slugs it is
// given. This proves that path cannot strip a protected plugin — the MCP builder
// tool now enforces the same policy as the UI disable endpoint (both consult
// pluginstore.PluginDetachLock via replacePlugins/protectedAgentPluginIDs).
func TestUpdateAgent_ProtectedPluginsSurvivePluginReplace(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	deps := noopDeps(db)
	ctx := context.Background()

	// --- default_agent_install: locked on the default agent, free elsewhere -----
	dap := seedInstalledPluginWithManifest(t, db, org.ID, "agent-builder", `{"default_agent_install":true}`)

	defAgent, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Default Assistant"})
	if err != nil {
		t.Fatalf("create default agent: %v", err)
	}
	if err := db.Model(&model.Agent{}).Where("id = ?", defAgent.ID).Update("is_default", true).Error; err != nil {
		t.Fatalf("mark default: %v", err)
	}
	attachAgentPlugin(t, db, org.ID, defAgent.ID, dap.ID)

	if _, err := UpdateAgent(ctx, deps, org.ID, defAgent.ID, UpdateInput{SetPlugins: true, PluginIDs: nil}); err != nil {
		t.Fatalf("update default agent: %v", err)
	}
	if !agentPluginInstalled(db, defAgent.ID, dap.ID) {
		t.Fatal("default_agent_install plugin was stripped from the default agent")
	}

	// The same plugin on a NON-default agent is an ordinary opt-in and removable.
	sideAgent, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Side Agent"})
	if err != nil {
		t.Fatalf("create side agent: %v", err)
	}
	attachAgentPlugin(t, db, org.ID, sideAgent.ID, dap.ID)
	if _, err := UpdateAgent(ctx, deps, org.ID, sideAgent.ID, UpdateInput{SetPlugins: true, PluginIDs: nil}); err != nil {
		t.Fatalf("update side agent: %v", err)
	}
	if agentPluginInstalled(db, sideAgent.ID, dap.ID) {
		t.Fatal("a default_agent_install plugin should be removable from a non-default agent")
	}

	// --- catalog-required: locked on the requiring agent; opt-in is free --------
	req := seedInstalledPluginWithManifest(t, db, org.ID, "github", `{}`)
	extra := seedInstalledPluginWithManifest(t, db, org.ID, "extra", `{}`)
	catalog := model.AgentCatalog{
		ID:              uuid.New(),
		Slug:            "cat-" + uuid.NewString()[:8],
		Name:            "Catalog",
		Status:          model.AgentCatalogStatusActive,
		RequiredPlugins: pq.StringArray{req.Slug},
	}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{}) })

	catAgent, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Catalog Agent"})
	if err != nil {
		t.Fatalf("create catalog agent: %v", err)
	}
	if err := db.Model(&model.Agent{}).Where("id = ?", catAgent.ID).Update("agent_catalog_id", catalog.ID).Error; err != nil {
		t.Fatalf("bind catalog: %v", err)
	}
	attachAgentPlugin(t, db, org.ID, catAgent.ID, req.ID)
	attachAgentPlugin(t, db, org.ID, catAgent.ID, extra.ID)

	if _, err := UpdateAgent(ctx, deps, org.ID, catAgent.ID, UpdateInput{SetPlugins: true, PluginIDs: nil}); err != nil {
		t.Fatalf("update catalog agent: %v", err)
	}
	if !agentPluginInstalled(db, catAgent.ID, req.ID) {
		t.Fatal("catalog-required plugin was stripped by update_agent")
	}
	if agentPluginInstalled(db, catAgent.ID, extra.ID) {
		t.Fatal("opt-in plugin should have been removed by the empty replace")
	}
}
