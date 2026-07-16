package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestDatabaseIntegrationCreateStoresOrgScopedConnection(t *testing.T) {
	db := connectTestDB(t)
	kms := newTestKMS(t)
	h := handler.NewDatabaseIntegrationHandler(db, kms)

	org := createDatabaseScopeTestOrg(t, db)
	agent := createDatabaseScopeTestAgent(t, db, org.ID, "create")

	body, _ := json.Marshal(map[string]any{
		"provider":       "postgres",
		"display_name":   "Reporting DB",
		"connection_url": testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL"),
		"agent_id":       agent.ID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/database-integrations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["agent_id"]; ok {
		t.Fatalf("response included agent_id: %s", rr.Body.String())
	}

	var count int64
	if err := db.Model(&model.DatabaseConnection{}).
		Where("org_id = ? AND provider = ? AND revoked_at IS NULL", org.ID, "postgres").
		Count(&count).Error; err != nil {
		t.Fatalf("count database connections: %v", err)
	}
	if count != 1 {
		t.Fatalf("database connections = %d, want 1", count)
	}
}

func TestDatabaseIntegrationCreateAllowsSecondProviderConnectionAndRequiresName(t *testing.T) {
	db := connectTestDB(t)
	kms := newTestKMS(t)
	h := handler.NewDatabaseIntegrationHandler(db, kms)
	org := createDatabaseScopeTestOrg(t, db)

	for index := 0; index < 2; index++ {
		body, _ := json.Marshal(map[string]any{
			"provider":       "postgres",
			"display_name":   "Postgres",
			"connection_url": testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL"),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/database-integrations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		recorder := httptest.NewRecorder()
		h.Create(recorder, req)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("create %d status = %d: %s", index+1, recorder.Code, recorder.Body.String())
		}
	}

	var connections []model.DatabaseConnection
	if err := db.Where("org_id = ? AND provider = ? AND revoked_at IS NULL", org.ID, "postgres").
		Order("created_at ASC").Find(&connections).Error; err != nil {
		t.Fatalf("load database connections: %v", err)
	}
	if len(connections) != 2 {
		t.Fatalf("connections = %d, want 2", len(connections))
	}
	if connections[0].Slug != "postgres" || connections[0].NeedsName {
		t.Fatalf("first identity = %#v", connections[0])
	}
	if len(connections[1].Name) != 6 || connections[1].Slug != connections[1].Name || !connections[1].NeedsName {
		t.Fatalf("second identity = name %q slug %q needs_name %v", connections[1].Name, connections[1].Slug, connections[1].NeedsName)
	}
}

func TestDatabaseProxyUsesPluginInstallForOrgScopedCredential(t *testing.T) {
	db := connectTestDB(t)
	kms := newTestKMS(t)
	encKey := testSymmetricKey(t)
	proxy := handler.NewDatabaseProxyHandler(db, encKey, kms)

	org := createDatabaseScopeTestOrg(t, db)
	allowedAgent := createDatabaseScopeTestAgent(t, db, org.ID, "allowed")
	deniedAgent := createDatabaseScopeTestAgent(t, db, org.ID, "denied")
	allowedSecret := createDatabaseScopeTestSandbox(t, db, encKey, org.ID, allowedAgent.ID)
	deniedSecret := createDatabaseScopeTestSandbox(t, db, encKey, org.ID, deniedAgent.ID)
	installDatabaseScopePlugin(t, db, org.ID, allowedAgent.ID, "postgres")
	createDatabaseScopeConnection(t, db, kms, org.ID, "postgres")

	r := chi.NewRouter()
	r.Post("/internal/database-proxy/postgres/{agentID}", proxy.Handle("postgres"))

	allowed := postDatabaseProxy(t, r, allowedAgent.ID, allowedSecret)
	if allowed.Code != http.StatusOK {
		t.Fatalf("allowed agent got %d: %s", allowed.Code, allowed.Body.String())
	}

	denied := postDatabaseProxy(t, r, deniedAgent.ID, deniedSecret)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("denied agent got %d: %s", denied.Code, denied.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(denied.Body).Decode(&resp); err != nil {
		t.Fatalf("decode denied response: %v", err)
	}
	if resp["error"] != "database connection not found" {
		t.Fatalf("denied error = %q, want database connection not found", resp["error"])
	}
}

func createDatabaseScopeTestOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("database-scope-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("org_id = ?", org.ID).Delete(&model.DatabaseConnection{})
		db.Where("org_id = ?", org.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return org
}

func createDatabaseScopeTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, label string) model.Agent {
	t.Helper()
	// Each agent gets its own team so a plugin grant to one agent's team never
	// leaks to the other (plugins resolve per team).
	team := model.Team{ID: uuid.New(), OrgID: orgID, Name: "database-scope-" + label + "-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{
		ID:     uuid.New(),
		OrgID:  &orgID,
		TeamID: team.ID,
		Name:   "database-scope-" + label + "-" + uuid.New().String()[:8],
		Model:  "test-model",
		Status: "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func createDatabaseScopeTestSandbox(t *testing.T, db *gorm.DB, encKey interface {
	EncryptString(string) ([]byte, error)
}, orgID, agentID uuid.UUID) string {
	t.Helper()
	runtimeSecret := "database-scope-runtime-" + uuid.NewString()
	encryptedRuntimeSecret, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &orgID,
		AgentID:                &agentID,
		EncryptedRuntimeSecret: encryptedRuntimeSecret,
		Status:                 "running",
		ExternalID:             "database-scope-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:25434",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return runtimeSecret
}

func installDatabaseScopePlugin(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, provider string) {
	t.Helper()
	pluginID := uuid.New()
	plugin := model.Plugin{
		ID:     pluginID,
		Slug:   "database-scope-" + provider + "-" + uuid.New().String()[:8],
		Name:   provider,
		Status: model.PluginStatusActive,
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	if err := db.Create(&model.PluginIntegration{
		PluginID: pluginID,
		Provider: provider,
		Kind:     model.PluginIntegrationKindDatabase,
		Required: true,
	}).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	if err := db.Create(&model.OrgPluginInstall{
		ID:       uuid.New(),
		OrgID:    orgID,
		PluginID: pluginID,
	}).Error; err != nil {
		t.Fatalf("create org plugin install: %v", err)
	}
	grantPluginToAgentTeam(t, db, orgID, agentID, pluginID)
	t.Cleanup(func() {
		db.Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ? AND plugin_id = ?", orgID, pluginID).Delete(&model.OrgPluginInstall{})
		db.Where("plugin_id = ?", pluginID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", pluginID).Delete(&model.Plugin{})
	})
}

func createDatabaseScopeConnection(t *testing.T, db *gorm.DB, kms *crypto.KeyWrapper, orgID uuid.UUID, provider string) {
	t.Helper()
	encryptedDSN, wrappedDEK, err := dbi.EncryptSecret(t.Context(), kms, testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("encrypt database dsn: %v", err)
	}
	conn := model.DatabaseConnection{
		ID:             uuid.New(),
		OrgID:          orgID,
		Provider:       provider,
		DisplayName:    provider,
		EncryptedDSN:   encryptedDSN,
		WrappedDEK:     wrappedDEK,
		SchemaSnapshot: model.RawJSON("{}"),
		AccessPolicy:   dbi.PolicyToJSON(dbi.Policy{MaxRows: 10}),
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create database connection: %v", err)
	}
}

func postDatabaseProxy(t *testing.T, r http.Handler, agentID uuid.UUID, secret string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/database-proxy/postgres/"+agentID.String(), bytes.NewBufferString("SELECT 1 AS ok"))
	req.Header.Set("Authorization", "Bearer "+secret)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}
