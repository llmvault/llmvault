package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGrafanaServiceUsesPinnedSpecAndReviewedReadActions(t *testing.T) {
	service := grafanaService()

	if !strings.Contains(service.SpecSource, grafanaSpecRevision) {
		t.Fatalf("Grafana spec source %q does not contain pinned revision %q", service.SpecSource, grafanaSpecRevision)
	}
	if len(service.OperationSelectors) != 4 {
		t.Fatalf("Grafana operation selectors = %d, want 4", len(service.OperationSelectors))
	}
	if len(service.ActionOverrides) != 4 {
		t.Fatalf("Grafana action overrides = %d, want 4", len(service.ActionOverrides))
	}

	var allowedDeprecated bool
	for _, selector := range service.OperationSelectors {
		if selector.Path == "/dashboards/uid/{uid}" && selector.Method == "GET" {
			allowedDeprecated = selector.AllowDeprecated
		}
	}
	if !allowedDeprecated {
		t.Fatal("Grafana dashboard read endpoint must carry an explicit deprecated-endpoint exception")
	}

	for operationID, override := range service.ActionOverrides {
		if override.Key == "" || override.DisplayName == "" || override.Description == "" {
			t.Errorf("Grafana override %s has incomplete tool documentation: %#v", operationID, override)
		}
		if override.Access != "read" {
			t.Errorf("Grafana override %s access = %q, want read", operationID, override.Access)
		}
		if len(override.Parameters) > 0 && !json.Valid(override.Parameters) {
			t.Errorf("Grafana override %s has invalid parameter JSON", operationID)
		}
	}
}
