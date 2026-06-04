package railway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreateServiceFromImageUsesGraphQLTransport(t *testing.T) {
	var capturedAuth string
	var captured struct {
		OperationName string         `json:"operationName"`
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"serviceCreate":{"id":"svc_123","name":"warm-1"}}}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{Endpoint: server.URL, Token: "railway-token", HTTP: server.Client()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	service, err := client.CreateServiceFromImage(t.Context(), CreateServiceInput{
		Name:          "warm-1",
		ProjectID:     "project",
		EnvironmentID: "env",
		Image:         "runtime:test",
		Variables:     map[string]string{"HIVY_RUNTIME_SECRET": "bootstrap"},
	})
	if err != nil {
		t.Fatalf("CreateServiceFromImage: %v", err)
	}
	if service.ID != "svc_123" || service.Name != "warm-1" {
		t.Fatalf("service = %#v", service)
	}
	if capturedAuth != "Bearer railway-token" {
		t.Fatalf("Authorization = %q", capturedAuth)
	}
	if captured.OperationName != "CreateRailwaySandboxService" {
		t.Fatalf("operation = %q", captured.OperationName)
	}
	if captured.Variables["projectId"] != "project" || captured.Variables["environmentId"] != "env" {
		t.Fatalf("variables = %#v", captured.Variables)
	}
}

func TestClientRestartLatestDeploymentRestartsNewestDeployment(t *testing.T) {
	var requests []struct {
		OperationName string         `json:"operationName"`
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var captured struct {
			OperationName string         `json:"operationName"`
			Query         string         `json:"query"`
			Variables     map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, captured)
		w.Header().Set("Content-Type", "application/json")
		switch captured.OperationName {
		case "RailwaySandboxDeployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":{"edges":[{"node":{"id":"dep_123","status":"SUCCESS","createdAt":"2026-06-03T00:00:00Z","updatedAt":"2026-06-03T00:00:00Z"}}]}}}`))
		case "RestartRailwaySandboxDeployment":
			_, _ = w.Write([]byte(`{"data":{"deploymentRestart":true}}`))
		default:
			t.Fatalf("unexpected operation %q", captured.OperationName)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Config{Endpoint: server.URL, Token: "railway-token", HTTP: server.Client()})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.RestartLatestDeployment(t.Context(), DeploymentListInput{
		ProjectID:     "project",
		EnvironmentID: "env",
		ServiceID:     "svc_123",
		First:         1,
	}); err != nil {
		t.Fatalf("RestartLatestDeployment: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if requests[0].OperationName != "RailwaySandboxDeployments" {
		t.Fatalf("first operation = %q", requests[0].OperationName)
	}
	if requests[1].OperationName != "RestartRailwaySandboxDeployment" {
		t.Fatalf("second operation = %q", requests[1].OperationName)
	}
	if requests[1].Variables["id"] != "dep_123" {
		t.Fatalf("restart variables = %#v", requests[1].Variables)
	}
}
