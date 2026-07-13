package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegrationHandler_List_Success(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	providers := []string{"github", "slack", "notion"}
	for _, p := range providers {
		integ := model.Integration{
			ID:          uuid.New(),
			UniqueKey:   fmt.Sprintf("%s-%s", p, uuid.New().String()[:8]),
			Provider:    p,
			DisplayName: p + " test",
		}
		if err := h.db.Create(&integ).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations", nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) < 3 {
		t.Fatalf("expected at least 3 integrations, got %d", len(page.Data))
	}
}

func TestIntegrationHandler_List_ExcludesDeleted(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	integ := createTestIntegration(t, h.db, "github")
	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations", nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["id"] == integ.ID.String() {
			t.Fatal("deleted integration should not appear in list")
		}
	}
}

func TestIntegrationHandler_List_Pagination(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	for i := 0; i < 5; i++ {
		provider := fmt.Sprintf("provider-%d-%s", i, uuid.New().String()[:8])
		integ := model.Integration{
			ID:          uuid.New(),
			UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
			Provider:    provider,
			DisplayName: provider,
		}
		if err := h.db.Create(&integ).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
		time.Sleep(time.Millisecond)
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations?limit=2", nil, &user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page1)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true")
	}
	if page1.NextCursor == nil {
		t.Fatal("expected next_cursor to be present")
	}

	rr2 := h.doRequest(t, http.MethodGet, "/v1/integrations?limit=2&cursor="+*page1.NextCursor, nil, &user)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}

	var page2 struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr2.Body).Decode(&page2)
	if len(page2.Data) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Data))
	}
}

func TestIntegrationHandler_ListAvailable_Success(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	createTestIntegration(t, h.db, "notion")

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) < 1 {
		t.Fatal("expected at least 1 available integration")
	}

	for _, item := range resp {
		if _, exists := item["unique_key"]; exists {
			t.Fatal("unique_key should not be in available response")
		}
	}
}

func TestIntegrationHandler_ListSupported_IncludesUnconfiguredDefinitions(t *testing.T) {
	h := newIntegrationHarness(t, nil)

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations/supported", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []struct {
			DefinitionID string `json:"definition_id"`
			Configured   bool   `json:"configured"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode supported integrations: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected supported integration definitions")
	}
	for _, item := range resp.Data {
		if item.DefinitionID == "slack" {
			if item.Configured {
				t.Fatal("unconfigured Slack definition reported as configured")
			}
			return
		}
	}
	t.Fatal("supported integrations missing Slack")
}

func TestIntegrationHandler_ListAvailable_ReturnsSanitizedNangoConfig(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	bugsink := createTestIntegration(t, h.db, "bugsink")
	github := createTestIntegration(t, h.db, "github-app")
	h.db.Model(&bugsink).Update("nango_config", model.JSON{
		"auth_mode": "API_KEY",
		"connection_config": map[string]any{
			"baseUrl": map[string]any{
				"title":       "Bugsink URL",
				"description": "Base URL for the Bugsink instance.",
				"type":        "string",
				"example":     "https://bugsink.example.com",
			},
		},
		"credentials_schema":          map[string]any{"api_key": map[string]any{"type": "string"}},
		"webhook_routing_script":      "secret script",
		"webhook_secret":              "secret",
		"webhook_url":                 "https://hooks.example.test",
		"webhook_user_defined_secret": true,
	})
	h.db.Model(&github).Update("nango_config", model.JSON{
		"auth_mode": "APP",
		"connection_config": map[string]any{
			"appPublicLink": map[string]any{
				"title":       "GitHub App link",
				"description": "GitHub App public link.",
				"type":        "string",
				"automated":   true,
			},
			"installation_id": map[string]any{
				"title":       "Installation ID",
				"description": "GitHub App installation ID.",
				"type":        "string",
				"automated":   true,
			},
		},
	})

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	byProvider := availableByProvider(resp)

	bugsinkConfig := requireNangoConfig(t, byProvider["bugsink"])
	if bugsinkConfig["auth_mode"] != "API_KEY" {
		t.Fatalf("bugsink auth_mode = %v, want API_KEY", bugsinkConfig["auth_mode"])
	}
	connectionConfig := bugsinkConfig["connection_config"].(map[string]any)
	if _, ok := connectionConfig["baseUrl"]; !ok {
		t.Fatalf("bugsink connection_config missing baseUrl: %v", connectionConfig)
	}
	for _, secretField := range []string{"credentials_schema", "webhook_routing_script", "webhook_secret", "webhook_url", "webhook_user_defined_secret"} {
		if _, ok := bugsinkConfig[secretField]; ok {
			t.Fatalf("available nango_config leaked %s", secretField)
		}
	}

	githubConfig := requireNangoConfig(t, byProvider["github-app"])
	if githubConfig["auth_mode"] != "APP" {
		t.Fatalf("github auth_mode = %v, want APP", githubConfig["auth_mode"])
	}
	githubConnectionConfig := githubConfig["connection_config"].(map[string]any)
	appPublicLink := githubConnectionConfig["appPublicLink"].(map[string]any)
	if appPublicLink["automated"] != true {
		t.Fatalf("github appPublicLink automated = %v, want true", appPublicLink["automated"])
	}
}

func TestIntegrationHandler_ListAvailable_RefreshesMissingNangoConfig(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	integ := createTestIntegration(t, h.db, "glitchtip")

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	glitchtipConfig := requireNangoConfig(t, availableByProvider(resp)["glitchtip"])
	if glitchtipConfig["auth_mode"] != "API_KEY" {
		t.Fatalf("glitchtip auth_mode = %v, want API_KEY", glitchtipConfig["auth_mode"])
	}
	connectionConfig := glitchtipConfig["connection_config"].(map[string]any)
	if _, ok := connectionConfig["baseUrl"]; !ok {
		t.Fatalf("glitchtip connection_config missing baseUrl: %v", connectionConfig)
	}

	var stored model.Integration
	if err := h.db.First(&stored, "id = ?", integ.ID).Error; err != nil {
		t.Fatalf("load integration: %v", err)
	}
	if stored.NangoConfig["auth_mode"] != "API_KEY" {
		t.Fatalf("stored auth_mode = %v, want API_KEY", stored.NangoConfig["auth_mode"])
	}
}

func TestIntegrationHandler_ListAvailable_ExcludesDeleted(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	integ := createTestIntegration(t, h.db, "notion")

	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	for _, item := range resp {
		if item["id"] == integ.ID.String() {
			t.Fatal("deleted integration should not appear in available list")
		}
	}
}

func availableByProvider(resp []map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(resp))
	for _, item := range resp {
		provider, _ := item["provider"].(string)
		out[provider] = item
	}
	return out
}

func requireNangoConfig(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	if item == nil {
		t.Fatal("available integration not found")
	}
	cfg, ok := item["nango_config"].(map[string]any)
	if !ok {
		t.Fatalf("nango_config missing or invalid: %v", item["nango_config"])
	}
	return cfg
}
