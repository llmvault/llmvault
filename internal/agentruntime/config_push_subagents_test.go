package agentruntime

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// End-to-end confirmation that a user agent's sub-agents survive compilation and
// the real runtime-config push: we compile a parent with two DB-backed
// sub-agents, PUT the config through the actual Client (gzip transport), and
// assert the sub-agents (and their scoped tools) arrive in the pushed payload.
func TestPutRuntimeConfig_PushesCompiledSubAgents(t *testing.T) {
	db := connectCompileTestDB(t)
	org := createOrg(t, db)
	parent := createUserAgent(t, db, org.ID, "Aria")
	createSubAgent(t, db, org.ID, parent.ID, subAgentSeed{
		Name:         "Researcher",
		Instructions: "Find sources.",
		Tools:        model.JSON{"bash": true},
	})
	createSubAgent(t, db, org.ID, parent.ID, subAgentSeed{
		Name:         "Writer",
		Instructions: "Draft output.",
		Tools:        model.JSON{"write_file": true},
	})

	def, err := Compile(context.Background(), CompileDeps{DB: db, Cfg: &config.Config{}}, &parent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(def.SubAgents) != 2 {
		t.Fatalf("compiled sub_agents = %d, want 2", len(def.SubAgents))
	}

	seen := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config" || r.Method != http.MethodPut {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
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
			t.Errorf("decode config body: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		seen <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"env_key_count":0}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "runtime-secret")
	if _, err := client.PutRuntimeConfig(context.Background(), ConfigUpdateRequest{Definition: def}); err != nil {
		t.Fatalf("PutRuntimeConfig: %v", err)
	}

	body := <-seen
	definition, ok := body["definition"].(map[string]any)
	if !ok {
		t.Fatalf("definition missing in pushed body: %#v", body)
	}
	subAgents, ok := definition["sub_agents"].(map[string]any)
	if !ok || len(subAgents) != 2 {
		t.Fatalf("pushed sub_agents = %#v, want 2", definition["sub_agents"])
	}

	toolTypesByName := map[string][]string{}
	for _, raw := range subAgents {
		sub, _ := raw.(map[string]any)
		meta, _ := sub["agent"].(map[string]any)
		name, _ := meta["name"].(string)
		for _, toolRaw := range asAnySlice(sub["tools"]) {
			tool, _ := toolRaw.(map[string]any)
			if typ, _ := tool["type"].(string); typ != "" {
				toolTypesByName[name] = append(toolTypesByName[name], typ)
			}
		}
	}
	if _, ok := toolTypesByName["Researcher"]; !ok {
		t.Fatalf("pushed sub-agents missing Researcher: %#v", toolTypesByName)
	}
	if _, ok := toolTypesByName["Writer"]; !ok {
		t.Fatalf("pushed sub-agents missing Writer: %#v", toolTypesByName)
	}
	if !containsString(toolTypesByName["Researcher"], "builtin.bash") {
		t.Fatalf("Researcher tools = %#v, want builtin.bash", toolTypesByName["Researcher"])
	}
	if !containsString(toolTypesByName["Writer"], "builtin.write_file") {
		t.Fatalf("Writer tools = %#v, want builtin.write_file", toolTypesByName["Writer"])
	}
}

func asAnySlice(value any) []any {
	if s, ok := value.([]any); ok {
		return s
	}
	return nil
}
