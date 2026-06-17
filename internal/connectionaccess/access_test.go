package connectionaccess

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestEffectiveResourcesPrefersAgentOverride(t *testing.T) {
	connID := uuid.New()
	conn := model.Connection{
		ID: connID,
		Meta: model.JSON{"resources": map[string]any{
			"repository": []any{map[string]any{"id": "org/default", "name": "default"}},
		}},
	}
	agentResources := model.JSON{
		connID.String(): model.JSON{
			"repository": []any{map[string]any{"id": "org/override", "name": "override"}},
		},
	}

	got := EffectiveResources(agentResources, conn)
	repos, ok := got["repository"].([]any)
	if !ok || len(repos) != 1 {
		t.Fatalf("repository resources = %#v", got["repository"])
	}
	repo, ok := repos[0].(map[string]any)
	if !ok || repo["id"] != "org/override" {
		t.Fatalf("expected agent override resource, got %#v", repos[0])
	}
}

func TestEffectiveResourcesFallsBackToConnectionMeta(t *testing.T) {
	conn := model.Connection{
		ID: uuid.New(),
		Meta: model.JSON{"resources": map[string]any{
			"repository": []any{map[string]any{"id": "org/default", "name": "default"}},
		}},
	}

	got := EffectiveResources(model.JSON{}, conn)
	repos, ok := got["repository"].([]any)
	if !ok || len(repos) != 1 {
		t.Fatalf("repository resources = %#v", got["repository"])
	}
	repo, ok := repos[0].(map[string]any)
	if !ok || repo["id"] != "org/default" {
		t.Fatalf("expected connection default resource, got %#v", repos[0])
	}
}

func TestEffectiveResourcesReturnsEmptyWhenUnrestricted(t *testing.T) {
	got := EffectiveResources(model.JSON{}, model.Connection{ID: uuid.New(), Meta: model.JSON{}})
	if len(got) != 0 {
		t.Fatalf("expected empty unrestricted resources, got %#v", got)
	}
}

func TestResolveAgentProviderRequiresEnabledPlugin(t *testing.T) {
	db := newResolverTestDB(t)
	fixture := insertResolverFixture(t, db)

	_, err := ResolveAgentProvider(context.Background(), db, fixture.orgID, fixture.agentID, "linear")
	if err == nil {
		t.Fatal("expected resolver to deny provider without enabled plugin")
	}

	insertEnabledPlugin(t, db, fixture.orgID, fixture.agentID, "linear")
	result, err := ResolveAgentProvider(context.Background(), db, fixture.orgID, fixture.agentID, "linear")
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	if result.Connection.ID != fixture.connectionID {
		t.Fatalf("connection id = %s, want %s", result.Connection.ID, fixture.connectionID)
	}
	if result.ProviderConfigKey != fixture.integrationUniqueKey {
		t.Fatalf("provider config key = %q, want %q", result.ProviderConfigKey, fixture.integrationUniqueKey)
	}
}

func TestResolveAgentProviderUsesEffectiveResources(t *testing.T) {
	db := newResolverTestDB(t)
	fixture := insertResolverFixture(t, db)
	insertEnabledPlugin(t, db, fixture.orgID, fixture.agentID, "linear")
	if err := db.Model(&model.Connection{}).Where("id = ?", fixture.connectionID).Update("meta", model.JSON{
		"resources": map[string]any{"project": []any{map[string]any{"id": "default", "name": "Default"}}},
	}).Error; err != nil {
		t.Fatalf("update connection meta: %v", err)
	}
	if err := db.Model(&model.Agent{}).Where("id = ?", fixture.agentID).Update("resources", model.JSON{
		fixture.connectionID.String(): model.JSON{"project": []any{map[string]any{"id": "override", "name": "Override"}}},
	}).Error; err != nil {
		t.Fatalf("update agent resources: %v", err)
	}

	result, err := ResolveAgentProvider(context.Background(), db, fixture.orgID, fixture.agentID, "linear")
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	projects, ok := result.Resources["project"].([]any)
	if !ok || len(projects) != 1 {
		t.Fatalf("project resources = %#v", result.Resources["project"])
	}
	project, ok := projects[0].(map[string]any)
	if !ok || project["id"] != "override" {
		t.Fatalf("expected agent override resource, got %#v", projects[0])
	}
}

type resolverFixture struct {
	orgID                uuid.UUID
	userID               uuid.UUID
	agentID              uuid.UUID
	integrationID        uuid.UUID
	integrationUniqueKey string
	connectionID         uuid.UUID
}

func newResolverTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func insertResolverFixture(t *testing.T, db *gorm.DB) resolverFixture {
	t.Helper()
	fixture := resolverFixture{
		orgID:                uuid.New(),
		userID:               uuid.New(),
		agentID:              uuid.New(),
		integrationID:        uuid.New(),
		integrationUniqueKey: "linear-" + uuid.NewString()[:8],
		connectionID:         uuid.New(),
	}
	if err := db.Create(&model.Org{ID: fixture.orgID, Name: "resolver-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if err := db.Create(&model.User{ID: fixture.userID, Email: "resolver-" + uuid.NewString()[:8] + "@example.com"}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	description := ""
	if err := db.Create(&model.Agent{
		ID:          fixture.agentID,
		OrgID:       &fixture.orgID,
		Name:        "resolver-agent-" + uuid.NewString()[:8],
		Description: &description,
		Model:       "test-model",
		Status:      "active",
		Resources:   model.JSON{},
	}).Error; err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if err := db.Create(&model.Integration{
		ID:          fixture.integrationID,
		UniqueKey:   fixture.integrationUniqueKey,
		Provider:    "linear",
		DisplayName: "Linear",
	}).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := db.Create(&model.Connection{
		ID:                fixture.connectionID,
		OrgID:             fixture.orgID,
		UserID:            fixture.userID,
		IntegrationID:     fixture.integrationID,
		NangoConnectionID: "nango-linear",
		Meta:              model.JSON{},
	}).Error; err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	return fixture
}

func insertEnabledPlugin(t *testing.T, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, provider string) {
	t.Helper()
	pluginID := uuid.New()
	if err := db.Create(&model.Plugin{
		ID:     pluginID,
		Slug:   "resolver-" + provider + "-" + uuid.NewString()[:8],
		Name:   "Resolver " + provider,
		Status: model.PluginStatusActive,
	}).Error; err != nil {
		t.Fatalf("insert plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
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
