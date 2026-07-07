package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	pluginstore "github.com/usehivy/hivy/internal/plugins"
)

// seedProviderPlugin installs an org plugin that requires the given provider via
// a plugin_integrations row, so the GitHub-identity guard can resolve it.
func seedProviderPlugin(t *testing.T, db *gorm.DB, orgID uuid.UUID, provider string) model.Plugin {
	t.Helper()
	plugin := seedInstalledPlugin(t, db, orgID, "excl-"+provider, "")
	if err := db.Create(&model.PluginIntegration{
		PluginID: plugin.ID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration %q: %v", provider, err)
	}
	t.Cleanup(func() { db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{}) })
	return plugin
}

func TestCreateAgent_RejectsBothGitHubPlugins(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	deps := noopDeps(db)
	primary := seedProviderPlugin(t, db, org.ID, "github-app")
	reviews := seedProviderPlugin(t, db, org.ID, "github-app-code-reviews")

	_, err := CreateAgent(context.Background(), deps, org.ID, CreateInput{
		Name:      "Both GitHub",
		PluginIDs: []uuid.UUID{primary.ID, reviews.ID},
	})
	if !errors.Is(err, pluginstore.ErrGitHubIdentityExclusive) {
		t.Fatalf("create with both GitHub plugins: got %v, want ErrGitHubIdentityExclusive", err)
	}
}

func TestUpdateAgent_RejectsSecondGitHubPluginBothOrders(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	deps := noopDeps(db)
	ctx := context.Background()
	primary := seedProviderPlugin(t, db, org.ID, "github-app")
	reviews := seedProviderPlugin(t, db, org.ID, "github-app-code-reviews")

	// Order A: agent has the primary GitHub plugin, update tries to add reviews.
	agentA, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Primary", PluginIDs: []uuid.UUID{primary.ID}})
	if err != nil {
		t.Fatalf("create agent A: %v", err)
	}
	_, err = UpdateAgent(ctx, deps, org.ID, agentA.ID, UpdateInput{
		SetPlugins: true,
		PluginIDs:  []uuid.UUID{primary.ID, reviews.ID},
	})
	if !errors.Is(err, pluginstore.ErrGitHubIdentityExclusive) {
		t.Fatalf("order A (add reviews): got %v, want ErrGitHubIdentityExclusive", err)
	}

	// Order B: agent has the reviews plugin, update tries to add the primary.
	agentB, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Reviews", PluginIDs: []uuid.UUID{reviews.ID}})
	if err != nil {
		t.Fatalf("create agent B: %v", err)
	}
	_, err = UpdateAgent(ctx, deps, org.ID, agentB.ID, UpdateInput{
		SetPlugins: true,
		PluginIDs:  []uuid.UUID{reviews.ID, primary.ID},
	})
	if !errors.Is(err, pluginstore.ErrGitHubIdentityExclusive) {
		t.Fatalf("order B (add primary): got %v, want ErrGitHubIdentityExclusive", err)
	}
}

func TestUpdateAgent_ReinstallSameGitHubPluginOK(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	deps := noopDeps(db)
	ctx := context.Background()
	primary := seedProviderPlugin(t, db, org.ID, "github-app")

	agent, err := CreateAgent(ctx, deps, org.ID, CreateInput{Name: "Primary", PluginIDs: []uuid.UUID{primary.ID}})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := UpdateAgent(ctx, deps, org.ID, agent.ID, UpdateInput{
		SetPlugins: true,
		PluginIDs:  []uuid.UUID{primary.ID},
	}); err != nil {
		t.Fatalf("re-setting the same GitHub plugin: unexpected error %v", err)
	}
	if !agentPluginInstalled(db, agent.ID, primary.ID) {
		t.Fatalf("primary GitHub plugin should still be installed")
	}
}

func TestCreateAgent_NonGitHubPluginsUnaffected(t *testing.T) {
	db := testDB(t)
	org := testOrg(t, db)
	deps := noopDeps(db)
	slack := seedProviderPlugin(t, db, org.ID, "slack")
	linear := seedProviderPlugin(t, db, org.ID, "linear")

	agent, err := CreateAgent(context.Background(), deps, org.ID, CreateInput{
		Name:      "Non GitHub",
		PluginIDs: []uuid.UUID{slack.ID, linear.ID},
	})
	if err != nil {
		t.Fatalf("create with two non-GitHub plugins: unexpected error %v", err)
	}
	if !agentPluginInstalled(db, agent.ID, slack.ID) || !agentPluginInstalled(db, agent.ID, linear.ID) {
		t.Fatalf("both non-GitHub plugins should be installed")
	}
}
