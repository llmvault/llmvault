package connectionaccess

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
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
		connID.String(): map[string]any{
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
	if result.ProviderConfigKey != "linear" {
		t.Fatalf("provider config key = %q", result.ProviderConfigKey)
	}
}

func TestResolveAgentProviderUsesEffectiveResources(t *testing.T) {
	db := newResolverTestDB(t)
	fixture := insertResolverFixture(t, db)
	insertEnabledPlugin(t, db, fixture.orgID, fixture.agentID, "linear")
	if err := db.Exec("UPDATE connections SET meta = ? WHERE id = ?", `{"resources":{"project":[{"id":"default","name":"Default"}]}}`, fixture.connectionID.String()).Error; err != nil {
		t.Fatalf("update connection meta: %v", err)
	}
	if err := db.Exec("UPDATE agents SET resources = ? WHERE id = ?", `{"`+fixture.connectionID.String()+`":{"project":[{"id":"override","name":"Override"}]}}`, fixture.agentID.String()).Error; err != nil {
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
	orgID         uuid.UUID
	agentID       uuid.UUID
	integrationID uuid.UUID
	connectionID  uuid.UUID
}

func newResolverTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE agents (id text primary key, org_id text, status text, resources json default '{}')`,
		`CREATE TABLE integrations (id text primary key, provider text, unique_key text, deleted_at timestamp null)`,
		`CREATE TABLE connections (id text primary key, org_id text, user_id text, integration_id text, nango_connection_id text, meta json default '{}', webhook_configured bool default true, revoked_at timestamp null, created_at timestamp, updated_at timestamp)`,
		`CREATE TABLE plugins (id text primary key, status text)`,
		`CREATE TABLE plugin_integrations (plugin_id text, provider text, kind text, required bool default true)`,
		`CREATE TABLE agent_plugin_installs (org_id text, agent_id text, plugin_id text)`,
		`CREATE TABLE org_plugin_installs (org_id text, plugin_id text, revoked_at timestamp null)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create resolver schema: %v", err)
		}
	}
	return db
}

func insertResolverFixture(t *testing.T, db *gorm.DB) resolverFixture {
	t.Helper()
	fixture := resolverFixture{
		orgID:         uuid.New(),
		agentID:       uuid.New(),
		integrationID: uuid.New(),
		connectionID:  uuid.New(),
	}
	if err := db.Exec("INSERT INTO agents (id, org_id, status, resources) VALUES (?, ?, 'active', '{}')", fixture.agentID.String(), fixture.orgID.String()).Error; err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if err := db.Exec("INSERT INTO integrations (id, provider, unique_key) VALUES (?, 'linear', 'linear')", fixture.integrationID.String()).Error; err != nil {
		t.Fatalf("insert integration: %v", err)
	}
	if err := db.Exec("INSERT INTO connections (id, org_id, integration_id, nango_connection_id, meta, created_at) VALUES (?, ?, ?, 'nango-linear', '{}', '2026-01-01T00:00:00Z')", fixture.connectionID.String(), fixture.orgID.String(), fixture.integrationID.String()).Error; err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	return fixture
}

func insertEnabledPlugin(t *testing.T, db *gorm.DB, orgID uuid.UUID, agentID uuid.UUID, provider string) {
	t.Helper()
	pluginID := uuid.New()
	if err := db.Exec("INSERT INTO plugins (id, status) VALUES (?, ?)", pluginID.String(), model.PluginStatusActive).Error; err != nil {
		t.Fatalf("insert plugin: %v", err)
	}
	if err := db.Exec("INSERT INTO plugin_integrations (plugin_id, provider, kind, required) VALUES (?, ?, ?, true)", pluginID.String(), provider, model.PluginIntegrationKindIntegration).Error; err != nil {
		t.Fatalf("insert plugin integration: %v", err)
	}
	if err := db.Exec("INSERT INTO org_plugin_installs (org_id, plugin_id) VALUES (?, ?)", orgID.String(), pluginID.String()).Error; err != nil {
		t.Fatalf("insert org plugin install: %v", err)
	}
	if err := db.Exec("INSERT INTO agent_plugin_installs (org_id, agent_id, plugin_id) VALUES (?, ?, ?)", orgID.String(), agentID.String(), pluginID.String()).Error; err != nil {
		t.Fatalf("insert agent plugin install: %v", err)
	}
}
