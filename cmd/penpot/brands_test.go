package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBrandsListCommandUsesRuntimeAuth(t *testing.T) {
	out := runBrandCommandRequest(t, []string{"brands", "list"}, http.MethodGet, "/internal/agents/agent-1/canvas/brands", nil, `{"data":[{"id":"brand-1"}],"has_more":false}`)
	if !strings.Contains(out, `"brand-1"`) {
		t.Fatalf("output missing brand id: %s", out)
	}
}

func TestBrandsViewCommandUsesRuntimeAuth(t *testing.T) {
	out := runBrandCommandRequest(t, []string{"brands", "view", "brand-1"}, http.MethodGet, "/internal/agents/agent-1/canvas/brands/brand-1", nil, `{"brand":{"id":"brand-1","name":"Acme"}}`)
	if !strings.Contains(out, `"Acme"`) {
		t.Fatalf("output missing brand name: %s", out)
	}
}

func TestBrandsCreateCommandSendsPayload(t *testing.T) {
	want := map[string]any{
		"name":        "Acme",
		"description": "Primary brand",
		"is_default":  true,
		"colors":      map[string]any{"tokens": []any{}},
	}
	runBrandCommandRequest(t,
		[]string{"brands", "create", "--name", "Acme", "--description", "Primary brand", "--default", "--json", `{"colors":{"tokens":[]}}`},
		http.MethodPost,
		"/internal/agents/agent-1/canvas/brands",
		want,
		`{"brand":{"id":"brand-1","name":"Acme"}}`,
	)
}

func TestBrandsUpdateCommandSendsPatchPayload(t *testing.T) {
	want := map[string]any{"description": "Updated"}
	runBrandCommandRequest(t,
		[]string{"brands", "update", "brand-1", "--json", `{"description":"Updated"}`},
		http.MethodPatch,
		"/internal/agents/agent-1/canvas/brands/brand-1",
		want,
		`{"brand":{"id":"brand-1","description":"Updated"}}`,
	)
}

func runBrandCommandRequest(t *testing.T, args []string, method, path string, wantBody map[string]any, response string) string {
	t.Helper()
	const runtimeSecret = "runtime-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			t.Errorf("method = %s, want %s", r.Method, method)
		}
		if r.URL.Path != path {
			t.Errorf("path = %s, want %s", r.URL.Path, path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+runtimeSecret {
			t.Errorf("authorization = %q", got)
		}
		if wantBody != nil {
			var got map[string]any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode body: %v", err)
			} else {
				assertJSONMapsEqual(t, got, wantBody)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	t.Setenv(envControlPlaneURL, server.URL)
	t.Setenv(envRuntimeSecret, runtimeSecret)
	t.Setenv(envAgentID, "agent-1")
	return captureStdout(t, func() error { return run(args) })
}

func assertJSONMapsEqual(t *testing.T, got, want map[string]any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("body = %s, want %s", gotJSON, wantJSON)
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	runErr := fn()
	_ = writer.Close()
	os.Stdout = old
	data, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	return string(data)
}
