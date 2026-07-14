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
	addTestOrgOwner(t, db, org.ID, user.ID)
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
	addTestOrgOwner(t, db, org.ID, user.ID)
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
