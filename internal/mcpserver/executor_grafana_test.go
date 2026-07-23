package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/nango"
)

func TestExecuteActionRunsGrafanaDataSourceQueryThroughNango(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/proxy/api/ds/query" {
			t.Errorf("Grafana proxy request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Provider-Config-Key"); got != "grafana" {
			t.Errorf("Provider-Config-Key = %q, want grafana", got)
		}
		if got := r.Header.Get("Connection-Id"); got != "grafana-connection-123" {
			t.Errorf("Connection-Id = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode Grafana query body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": map[string]any{
				"A": map[string]any{
					"frames": []any{map[string]any{
						"schema": map[string]any{"refId": "A"},
						"data":   map[string]any{"values": []any{[]any{1710000000000}, []any{42}}},
					}},
				},
			},
		})
	}))
	t.Cleanup(server.Close)

	provider, ok := catalog.Global().GetProvider("grafana")
	if !ok {
		t.Fatal("Grafana provider is missing")
	}
	action := provider.Actions["query_data_source"]
	params := map[string]any{
		"from": "now-1h",
		"to":   "now",
		"queries": []any{map[string]any{
			"refId":          "A",
			"datasource":     map[string]any{"uid": "prometheus-main", "type": "prometheus"},
			"expr":           `sum(rate(http_requests_total{job="api"}[5m]))`,
			"format":         "time_series",
			"maxDataPoints":  600,
			"intervalMs":     60000,
			"legendFormat":   "{{job}}",
			"datasourceName": "Production Prometheus",
		}},
	}

	result, err := ExecuteAction(
		context.Background(),
		nango.NewClient(server.URL, "nango-secret"),
		"grafana",
		"grafana",
		"grafana-connection-123",
		&action,
		params,
		nil,
	)
	if err != nil {
		t.Fatalf("execute Grafana query: %v", err)
	}

	queries := requestBody["queries"].([]any)
	query := queries[0].(map[string]any)
	if query["legendFormat"] != "{{job}}" || query["datasourceName"] != "Production Prometheus" {
		t.Fatalf("data-source-specific query fields were not preserved: %#v", query)
	}
	if result["results"] == nil {
		t.Fatalf("Grafana query result = %#v", result)
	}
}
