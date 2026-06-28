package agentruntime

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutRuntimeConfigSendsGzipBody(t *testing.T) {
	t.Parallel()

	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Encoding"); got != "gzip" {
			t.Errorf("Content-Encoding = %q, want gzip", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("gzip reader: %v", err)
			http.Error(w, "bad gzip", http.StatusBadRequest)
			return
		}
		defer reader.Close()
		var body map[string]any
		if err := json.NewDecoder(reader).Decode(&body); err != nil {
			t.Errorf("decode gzip JSON: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		seen <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"env_key_count":1}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "runtime-secret")
	_, err := client.PutRuntimeConfig(context.Background(), ConfigUpdateRequest{
		RuntimeEnv: map[string]string{"GOOD_KEY": "value"},
		Definition: &AgentDefinition{
			Agent:            AgentMeta{Name: "test"},
			Tools:            []map[string]any{},
			McpServers:       []any{},
			Skills:           []SkillSpec{},
			OutboundChannels: []any{},
		},
	})
	if err != nil {
		t.Fatalf("PutRuntimeConfig: %v", err)
	}

	body := <-seen
	runtimeEnv, ok := body["runtime_env"].(map[string]any)
	if !ok {
		t.Fatalf("runtime_env missing or wrong type: %#v", body["runtime_env"])
	}
	if got := runtimeEnv["GOOD_KEY"]; got != "value" {
		t.Fatalf("runtime_env GOOD_KEY = %#v, want value", got)
	}
}
