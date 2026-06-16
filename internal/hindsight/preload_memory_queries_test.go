package hindsight

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestPreloadMemoryListQueriesUseAgentInstalledPluginIntegrations(t *testing.T) {
	db, orgID, agent := newMemoryTagTestDB(t)
	addMemoryPluginInstall(t, db, orgID, agent.ID, "github-app")
	addUninstalledMemoryConnection(t, db, orgID, "slack")

	queries, err := PreloadMemoryListQueries(context.Background(), db, agent)
	if err != nil {
		t.Fatalf("preload queries: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("queries len = %d, want provider + org: %#v", len(queries), queries)
	}
	provider := queries[0]
	if provider.Name != "github-app" {
		t.Fatalf("provider query name = %q", provider.Name)
	}
	queryText := fmt.Sprintf("%#v", provider.TagGroups)
	for _, want := range []string{
		"scope:provider",
		"provider:github-app",
		"scope:resource",
		"resource_type:repository",
		"resource:github-app:repository:usehivy/usehivy.com",
	} {
		if !strings.Contains(queryText, want) {
			t.Fatalf("provider query missing %q: %#v", want, provider.TagGroups)
		}
	}
	if strings.Contains(queryText, "slack") {
		t.Fatalf("query should not include uninstalled provider: %#v", provider.TagGroups)
	}
	org := queries[1]
	if org.Name != "org" {
		t.Fatalf("org query name = %q", org.Name)
	}
	for _, want := range []string{"scope:provider", "scope:resource"} {
		if !hasString(org.ExcludeTags, want) {
			t.Fatalf("org query missing exclude tag %q: %#v", want, org.ExcludeTags)
		}
	}
}

func addMemoryPluginInstall(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, provider string) {
	t.Helper()
	pluginID := uuid.New()
	if err := db.Create(&model.Plugin{
		ID:       pluginID,
		Slug:     provider + "-plugin-" + uuid.NewString()[:8],
		Name:     provider,
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}).Error; err != nil {
		t.Fatalf("insert plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
	}).Error; err != nil {
		t.Fatalf("insert plugin integration: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{OrgID: orgID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("insert org plugin install: %v", err)
	}
	if err := db.Create(&model.AgentPluginInstall{OrgID: orgID, AgentID: agentID, PluginID: pluginID}).Error; err != nil {
		t.Fatalf("insert agent plugin install: %v", err)
	}
}

func addUninstalledMemoryConnection(t *testing.T, db *gorm.DB, orgID uuid.UUID, provider string) {
	t.Helper()
	userID := uuid.New()
	integrationID := uuid.New()
	connectionID := uuid.New()
	if err := db.Create(&model.User{ID: userID, Email: provider + "-" + uuid.NewString()[:8] + "@example.com"}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.Create(&model.Integration{
		ID:          integrationID,
		UniqueKey:   provider + "-" + uuid.NewString()[:8],
		Provider:    provider,
		DisplayName: provider,
	}).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := db.Create(&model.Connection{
		ID:                connectionID,
		OrgID:             orgID,
		UserID:            userID,
		IntegrationID:     integrationID,
		NangoConnectionID: provider + "-conn",
	}).Error; err != nil {
		t.Fatalf("insert connection: %v", err)
	}
}
