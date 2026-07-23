package nango

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyRequestEncodesQueryParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("query"); got != "API latency & errors" {
			t.Errorf("decoded query = %q", got)
		}
		if _, exists := r.URL.Query()[" errors"]; exists {
			t.Errorf("query value created an extra parameter: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	t.Cleanup(server.Close)

	_, err := NewClient(server.URL, "nango-secret").ProxyRequest(
		context.Background(),
		http.MethodGet,
		"grafana",
		"grafana-connection-123",
		"/api/search",
		map[string]string{"query": "API latency & errors"},
		nil,
	)
	if err != nil {
		t.Fatalf("proxy Grafana dashboard search: %v", err)
	}
}
