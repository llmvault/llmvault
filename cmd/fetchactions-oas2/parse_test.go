package main

import (
	"encoding/json"
	"testing"
)

func TestParseSpecBuildsOnlyReviewedGrafanaTools(t *testing.T) {
	spec := []byte(`{
	  "swagger": "2.0",
	  "basePath": "/api",
	  "paths": {
	    "/datasources": {
	      "get": {"operationId": "getDataSources", "responses": {"200": {"description": "ok"}}},
	      "post": {"operationId": "addDataSource", "responses": {"200": {"description": "ok"}}}
	    },
	    "/search": {
	      "get": {
	        "operationId": "search",
	        "parameters": [{"name": "query", "in": "query", "type": "string"}],
	        "responses": {"200": {"description": "ok"}}
	      }
	    },
	    "/dashboards/uid/{uid}": {
	      "get": {
	        "operationId": "getDashboardByUID",
	        "deprecated": true,
	        "parameters": [{"name": "uid", "in": "path", "required": true, "type": "string"}],
	        "responses": {"200": {"description": "ok"}}
	      },
	      "delete": {"operationId": "deleteDashboardByUID", "responses": {"200": {"description": "ok"}}}
	    },
	    "/ds/query": {
	      "post": {
	        "operationId": "queryMetricsWithExpressions",
	        "parameters": [{
	          "name": "body",
	          "in": "body",
	          "required": true,
	          "schema": {
	            "type": "object",
	            "required": ["from", "to", "queries"],
	            "properties": {
	              "from": {"type": "string"},
	              "to": {"type": "string"},
	              "queries": {"type": "array"},
	              "debug": {"type": "boolean"}
	            }
	          }
	        }],
	        "responses": {"200": {"description": "ok"}}
	      }
	    },
	    "/admin/users": {
	      "delete": {"operationId": "deleteAllUsers", "responses": {"200": {"description": "ok"}}
	    }
	  }
	}
	}`)

	result, err := parseSpec(spec, grafanaService())
	if err != nil {
		t.Fatalf("parse Grafana fixture: %v", err)
	}
	if len(result.Actions) != 4 {
		t.Fatalf("generated Grafana actions = %v, want exactly 4 reviewed tools", actionKeys(result.Actions))
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
		action, ok := result.Actions[key]
		if !ok {
			t.Errorf("generated Grafana catalog is missing %q", key)
			continue
		}
		if action.Access != "read" {
			t.Errorf("%s access = %q, want read", key, action.Access)
		}
		if action.Execution == nil || action.Execution.Method != expected.method || action.Execution.Path != expected.path {
			t.Errorf("%s execution = %#v, want %s %s", key, action.Execution, expected.method, expected.path)
		}
	}

	var querySchema map[string]any
	if err := json.Unmarshal(result.Actions["query_data_source"].Parameters, &querySchema); err != nil {
		t.Fatalf("decode query_data_source schema: %v", err)
	}
	properties := querySchema["properties"].(map[string]any)
	queries := properties["queries"].(map[string]any)
	if queries["minItems"] != float64(1) || queries["maxItems"] != float64(10) {
		t.Fatalf("query_data_source query bounds = %#v", queries)
	}
}

func actionKeys(actions map[string]ActionDef) []string {
	keys := make([]string, 0, len(actions))
	for key := range actions {
		keys = append(keys, key)
	}
	return keys
}
