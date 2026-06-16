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

	"github.com/usehivy/hivy/internal/agentruntime"
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

func TestConnectionHandler_Create_WithMeta(t *testing.T) {
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
	integ := createTestIntegration(t, db, "notion")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "nango-conn-meta",
		"meta": map[string]any{
			"label":     "docs connection",
			"resources": map[string]any{"repos": []string{"hivy"}},
		},
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
	meta, ok := resp["meta"].(map[string]any)
	if !ok || meta["label"] != "docs connection" {
		t.Fatalf("expected regular metadata to be preserved, got %v", resp["meta"])
	}
	if meta["resources"] != nil {
		t.Fatalf("expected meta.resources to be stripped, got %v", resp["meta"])
	}
}

func TestConnectionHandler_CreateGlitchTipStoresConnectionConfig(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.Connection{})
		db.Where("1=1").Delete(&model.Integration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{
		connectionConfig: map[string]any{"baseUrl": "https://app.glitchtip.com/"},
	}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewConnectionHandler(db, nangoClient, catalog.Global(), nil)
	r := chi.NewRouter()
	r.Post("/v1/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("glitchtip-conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, "glitchtip")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "glitchtip-conn-meta",
		"meta": map[string]any{
			"connection_config": map[string]any{"baseUrl": "https://app.glitchtip.com/"},
			"credentials":       map[string]any{"apiKey": "should-not-be-stored"},
		},
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
	meta, ok := resp["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata, got %v", resp["meta"])
	}
	connectionConfig, ok := meta["connection_config"].(map[string]any)
	if !ok {
		t.Fatalf("expected connection_config metadata, got %v", meta)
	}
	if got := connectionConfig["baseUrl"]; got != "https://app.glitchtip.com/" {
		t.Fatalf("baseUrl = %v, want GlitchTip base URL", got)
	}
	var conn model.Connection
	if err := db.Where("id = ?", resp["id"]).First(&conn).Error; err != nil {
		t.Fatalf("connection not found in DB: %v", err)
	}
	credentials, _ := conn.Meta["credentials"].(map[string]any)
	if _, ok := credentials["apiKey"]; ok {
		t.Fatal("created connection meta must not contain credentials.apiKey")
	}
	if got := agentruntime.GlitchTipDashboardBaseURLFromConnection(conn); got != "https://app.glitchtip.com" {
		t.Fatalf("dashboard base url = %q", got)
	}
}
