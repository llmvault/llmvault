package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestConnectionHandler_Create_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	addTestOrgOwner(t, db, org.ID, user.ID)
	integ := createTestIntegration(t, db, "notion")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "nango-conn-123",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["integration_id"] != integ.ID.String() {
		t.Fatalf("expected integration_id=%s, got %v", integ.ID.String(), resp["integration_id"])
	}
	if resp["provider"] != "notion" {
		t.Fatalf("expected provider=notion, got %v", resp["provider"])
	}
	if resp["nango_connection_id"] != "nango-conn-123" {
		t.Fatalf("expected nango_connection_id=nango-conn-123, got %v", resp["nango_connection_id"])
	}

	var conn model.Connection
	if err := db.Where("id = ?", resp["id"]).First(&conn).Error; err != nil {
		t.Fatalf("connection not found in DB: %v", err)
	}
	if conn.UserID != user.ID {
		t.Fatalf("expected user_id=%s, got %s", user.ID, conn.UserID)
	}
}

func TestConnectionHandler_Create_OnboardingInstallsAndEnablesMatchingPluginForSoleTeam(t *testing.T) {
	db := connectTestDB(t)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("onboarding-conn-%s@test.com", uuid.NewString()[:8]))
	org := createTestOrg(t, db)
	addTestOrgOwner(t, db, org.ID, user.ID)
	if err := db.Model(&model.Org{}).Where("id = ?", org.ID).Update("onboarding_step", model.OnboardingStepConnections).Error; err != nil {
		t.Fatalf("set onboarding step: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "onboarding-team-" + uuid.NewString()[:8]}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create onboarding team: %v", err)
	}
	integ := createTestIntegration(t, db, "github-app-code-reviews")
	plugin := model.Plugin{
		ID:       uuid.New(),
		Slug:     "onboarding-code-review-" + uuid.NewString()[:8],
		Name:     "Code Reviews",
		Status:   model.PluginStatusActive,
		Manifest: model.RawJSON(`{}`),
	}
	if err := db.Create(&plugin).Error; err != nil {
		t.Fatalf("create plugin: %v", err)
	}
	requirement := model.PluginIntegration{
		PluginID: plugin.ID,
		Provider: integ.Provider,
		Kind:     model.PluginIntegrationKindIntegration,
		Required: true,
	}
	if err := db.Create(&requirement).Error; err != nil {
		t.Fatalf("create plugin integration: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ? AND plugin_id = ?", org.ID, plugin.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ? AND plugin_id = ?", org.ID, plugin.ID).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", team.ID).Delete(&model.Team{})
		db.Where("plugin_id = ?", plugin.ID).Delete(&model.PluginIntegration{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.Connection{})
	})

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "onboarding-code-review-connection",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := db.Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", org.ID, plugin.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count plugin installs: %v", err)
	}
	if count != 1 {
		t.Fatalf("plugin install count = %d, want 1", count)
	}
	if err := db.Model(&model.TeamPlugin{}).
		Where("org_id = ? AND team_id = ? AND plugin_id = ?", org.ID, team.ID, plugin.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count team plugin grants: %v", err)
	}
	if count != 1 {
		t.Fatalf("team plugin grant count = %d, want 1", count)
	}

	normalUser := createTestUser(t, db, fmt.Sprintf("normal-conn-%s@test.com", uuid.NewString()[:8]))
	normalOrg := createTestOrg(t, db)
	addTestOrgOwner(t, db, normalOrg.ID, normalUser.ID)
	normalBody, _ := json.Marshal(map[string]any{
		"nango_connection_id": "normal-code-review-connection",
		"install_plugins":     true,
	})
	normalReq := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(normalBody))
	normalReq.Header.Set("Content-Type", "application/json")
	normalReq = middleware.WithUser(normalReq, &normalUser)
	normalReq = middleware.WithOrg(normalReq, &normalOrg)
	normalRecorder := httptest.NewRecorder()
	r.ServeHTTP(normalRecorder, normalReq)
	if normalRecorder.Code != http.StatusCreated {
		t.Fatalf("normal connection status = %d, want 201: %s", normalRecorder.Code, normalRecorder.Body.String())
	}
	if err := db.Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND plugin_id = ? AND revoked_at IS NULL", normalOrg.ID, plugin.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count normal-flow plugin installs: %v", err)
	}
	if count != 0 {
		t.Fatalf("normal-flow plugin install count = %d, want 0", count)
	}
}

func TestConnectionHandler_CreateSlackDoesNotCreateAgentSideEffects(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("slack-conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	addTestOrgOwner(t, db, org.ID, user.ID)
	integ := createTestIntegration(t, db, "slack")

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "slack-conn-123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var agentCount int64
	if err := db.Model(&model.Agent{}).Where("org_id = ? AND status <> ?", org.ID, "archived").Count(&agentCount).Error; err != nil {
		t.Fatalf("count agents: %v", err)
	}
	if agentCount != 0 {
		t.Fatalf("expected connection creation to avoid agent side effects, got %d agents", agentCount)
	}

	var triggerCount int64
	if err := db.Model(&model.AgentTrigger{}).Where("org_id = ?", org.ID).Count(&triggerCount).Error; err != nil {
		t.Fatalf("count triggers: %v", err)
	}
	if triggerCount != 0 {
		t.Fatalf("expected connection creation to avoid trigger side effects, got %d triggers", triggerCount)
	}
}

func TestConnectionHandler_Create_DuplicateUserIntegration(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	addTestOrgOwner(t, db, org.ID, user.ID)
	integ := createTestIntegration(t, db, "notion")

	db.Create(&model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: "first-conn",
	})

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "second-conn"})
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int64
	db.Model(&model.Connection{}).Where("user_id = ? AND integration_id = ?", user.ID, integ.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 connections, got %d", count)
	}
}
