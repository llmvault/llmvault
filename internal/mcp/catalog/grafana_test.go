package catalog

import (
	"encoding/json"
	"testing"
)

func TestGrafanaCatalogExposesReviewedReadTools(t *testing.T) {
	provider, ok := Global().GetProvider("grafana")
	if !ok {
		t.Fatal("Grafana provider is missing from the embedded catalog")
	}
	if provider.DisplayName != "Grafana" {
		t.Fatalf("Grafana display name = %q", provider.DisplayName)
	}
	if len(provider.Actions) != 4 {
		t.Fatalf("Grafana action count = %d, want 4", len(provider.Actions))
	}

	want := map[string]struct {
		method string
		path   string
	}{
		"list_data_sources": {"GET", "/api/datasources"},
		"search_dashboards": {"GET", "/api/search"},
		"get_dashboard":     {"GET", "/api/dashboards/uid/{uid}"},
		"query_data_source": {"POST", "/api/ds/query"},
	}
	for key, expected := range want {
		action, exists := provider.Actions[key]
		if !exists {
			t.Errorf("Grafana catalog is missing %q", key)
			continue
		}
		if action.Access != AccessRead {
			t.Errorf("%s access = %q, want read", key, action.Access)
		}
		if action.Description == "" {
			t.Errorf("%s has no agent-facing description", key)
		}
		if action.Execution == nil || action.Execution.Method != expected.method || action.Execution.Path != expected.path {
			t.Errorf("%s execution = %#v, want %s %s", key, action.Execution, expected.method, expected.path)
		}
	}

	var schema map[string]any
	if err := json.Unmarshal(provider.Actions["query_data_source"].Parameters, &schema); err != nil {
		t.Fatalf("decode query_data_source parameters: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	queries := properties["queries"].(map[string]any)
	items := queries["items"].(map[string]any)
	if items["additionalProperties"] != true {
		t.Fatal("query_data_source must preserve data-source-specific query fields")
	}
	if queries["maxItems"] != float64(10) {
		t.Fatalf("query_data_source maxItems = %v, want 10", queries["maxItems"])
	}
}
