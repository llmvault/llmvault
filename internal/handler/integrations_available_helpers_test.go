package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

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
