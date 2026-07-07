package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// seedProviderPlugin creates an active plugin that requires the given provider
// (via a plugin_integrations row), so exclusivity checks resolve its identity.
func seedProviderPlugin(t *testing.T, db *gorm.DB, provider string) model.Plugin {
	t.Helper()
	plugin := model.Plugin{
		ID:     uuid.New(),
		Slug:   "excl-" + provider + "-" + uuid.NewString()[:8],
		Name:   provider,
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin %q: %v", provider, err)
	}
	t.Cleanup(func() { db.Where("id = ?", plugin.ID).Delete(&model.Plugin{}) })
	if provider != "" {
		if err := db.Create(&model.PluginIntegration{
			PluginID: plugin.ID,
			Provider: provider,
			Kind:     model.PluginIntegrationKindIntegration,
			Required: true,
		}).Error; err != nil {
			t.Fatalf("create plugin integration %q: %v", provider, err)
		}
		t.Cleanup(func() { db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{}) })
	}
	return plugin
}

func seedExclusivityAgent(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "excl-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", org.ID).Delete(&model.Org{}) })
	agent := autoInstallTestAgent(org.ID, "excl")
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return org.ID, agent.ID
}

func attachExclusivityPlugin(t *testing.T, db *gorm.DB, orgID, agentID, pluginID uuid.UUID) {
	t.Helper()
	if err := db.Create(&model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("attach plugin: %v", err)
	}
}

func TestCheckGitHubIdentityExclusive_BothProvidersRejected(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	primary := seedProviderPlugin(t, db, "github-app")
	reviews := seedProviderPlugin(t, db, "github-app-code-reviews")

	err := CheckGitHubIdentityExclusive(context.Background(), db, []uuid.UUID{primary.ID, reviews.ID})
	if !errors.Is(err, ErrGitHubIdentityExclusive) {
		t.Fatalf("both GitHub providers: got %v, want ErrGitHubIdentityExclusive", err)
	}
}

func TestCheckGitHubIdentityExclusive_SingleProviderOK(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	primary := seedProviderPlugin(t, db, "github-app")
	other := seedProviderPlugin(t, db, "slack")

	if err := CheckGitHubIdentityExclusive(context.Background(), db, []uuid.UUID{primary.ID, other.ID}); err != nil {
		t.Fatalf("one GitHub provider plus a non-GitHub plugin: unexpected error %v", err)
	}
}

func TestCheckGitHubIdentityExclusiveOnAdd_SecondPluginRejectedBothOrders(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	ctx := context.Background()
	primary := seedProviderPlugin(t, db, "github-app")
	reviews := seedProviderPlugin(t, db, "github-app-code-reviews")

	// Order A: primary already installed, add reviews.
	orgA, agentA := seedExclusivityAgent(t, db)
	attachExclusivityPlugin(t, db, orgA, agentA, primary.ID)
	if err := CheckGitHubIdentityExclusiveOnAdd(ctx, db, orgA, agentA, reviews.ID); !errors.Is(err, ErrGitHubIdentityExclusive) {
		t.Fatalf("add reviews to primary: got %v, want ErrGitHubIdentityExclusive", err)
	}

	// Order B: reviews already installed, add primary.
	orgB, agentB := seedExclusivityAgent(t, db)
	attachExclusivityPlugin(t, db, orgB, agentB, reviews.ID)
	if err := CheckGitHubIdentityExclusiveOnAdd(ctx, db, orgB, agentB, primary.ID); !errors.Is(err, ErrGitHubIdentityExclusive) {
		t.Fatalf("add primary to reviews: got %v, want ErrGitHubIdentityExclusive", err)
	}
}

func TestCheckGitHubIdentityExclusiveOnAdd_ReinstallSamePluginOK(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	ctx := context.Background()
	primary := seedProviderPlugin(t, db, "github-app")

	orgID, agentID := seedExclusivityAgent(t, db)
	attachExclusivityPlugin(t, db, orgID, agentID, primary.ID)
	if err := CheckGitHubIdentityExclusiveOnAdd(ctx, db, orgID, agentID, primary.ID); err != nil {
		t.Fatalf("re-adding the already-installed GitHub plugin: unexpected error %v", err)
	}
}

func TestCheckGitHubIdentityExclusiveOnAdd_NonGitHubPluginsUnaffected(t *testing.T) {
	db := connectAutoInstallTestDB(t)
	ctx := context.Background()
	slack := seedProviderPlugin(t, db, "slack")
	linear := seedProviderPlugin(t, db, "linear")

	orgID, agentID := seedExclusivityAgent(t, db)
	attachExclusivityPlugin(t, db, orgID, agentID, slack.ID)
	if err := CheckGitHubIdentityExclusiveOnAdd(ctx, db, orgID, agentID, linear.ID); err != nil {
		t.Fatalf("two non-GitHub plugins: unexpected error %v", err)
	}
}
