package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestEnsureOpenRouterUsage_NonStreamingOmitsStreamOptions(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	if string(body["usage"]) != `{"include":true}` {
		t.Fatalf("usage = %s, want {\"include\":true}", body["usage"])
	}
	if _, ok := body["stream_options"]; ok {
		t.Fatalf("stream_options should be absent for non-streaming request")
	}
}

func TestEnsureOpenRouterUsage_MergesExistingStreamOptions(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","stream":true,"stream_options":{"other":1},"messages":[]}`)

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
	if string(so["other"]) != "1" {
		t.Fatalf("existing stream option lost: other = %s", so["other"])
	}
}

func TestEnsureOpenRouterUsage_PreservesModelAndMessages(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	if string(body["model"]) != `"z-ai/glm-5.2"` {
		t.Fatalf("model = %s, want \"z-ai/glm-5.2\"", body["model"])
	}
	if _, ok := body["messages"]; !ok {
		t.Fatalf("messages field lost")
	}
	if req.ContentLength <= 0 {
		t.Fatalf("content length not updated: %d", req.ContentLength)
	}
}

func TestEnsureOpenRouterUsage_InjectsEndUser(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenRouterUsage(req, "sess-123"); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	if string(body["user"]) != `"sess-123"` {
		t.Fatalf("user = %s, want \"sess-123\"", body["user"])
	}
}

func TestEnsureOpenRouterUsage_EmptyEndUserOmitsUser(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["user"]; ok {
		t.Fatalf("user field should be absent for empty end user")
	}
}

func TestEnsureOpenRouterUsage_DoesNotClobberExistingUser(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","user":"caller","messages":[]}`)

	if err := EnsureOpenRouterUsage(req, "sess-123"); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	if string(body["user"]) != `"caller"` {
		t.Fatalf("user = %s, want caller preserved", body["user"])
	}
}

func TestEnsureOpenRouterUsage_LeavesNonChatPathUntouched(t *testing.T) {
	body := `{"model":"z-ai/glm-5.2","input":"hi"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	raw, _ := io.ReadAll(req.Body)
	if string(raw) != body {
		t.Fatalf("body = %q, want untouched %q", raw, body)
	}
}

func TestEnsureOpenRouterUsage_LeavesNonJSONUntouched(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/v1/chat/completions", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "text/plain")

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != "not json" {
		t.Fatalf("body = %q, want untouched", raw)
	}
}
