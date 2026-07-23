package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func decodeBody(t *testing.T, req *http.Request) map[string]json.RawMessage {
	t.Helper()
	raw, _ := io.ReadAll(req.Body)
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode body %q: %v", raw, err)
	}
	return out
}

func TestEnsureOpenRouterUsage_StreamingInjectsIncludeUsage(t *testing.T) {
	req := makePostRequest(`{"model":"z-ai/glm-5.2","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenRouterUsage(req, ""); err != nil {
		t.Fatalf("EnsureOpenRouterUsage: %v", err)
	}

	body := decodeBody(t, req)
	if string(body["usage"]) != `{"include":true}` {
		t.Fatalf("usage = %s, want {\"include\":true}", body["usage"])
	}

	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

func TestEnsureOpenAICompatibleUsage_XiaomiStreamingContract(t *testing.T) {
	req := makePostRequest(`{"model":"mimo-v2.5-pro","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenAICompatibleUsage(req); err != nil {
		t.Fatalf("EnsureOpenAICompatibleUsage: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["usage"]; ok {
		t.Fatal("OpenRouter-only usage field must not be sent to Xiaomi")
	}
	if _, ok := body["user"]; ok {
		t.Fatal("OpenRouter-only user attribution must not be sent to Xiaomi")
	}

	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

func TestApplyUsageAccounting_XiaomiUsesOpenAICompatibleContract(t *testing.T) {
	req := makePostRequest(`{"model":"mimo-v2.5-pro","stream":true,"messages":[]}`)

	if err := applyUsageAccounting(req, "xiaomi", "https://api.xiaomimimo.com/v1", "ignored-user"); err != nil {
		t.Fatalf("applyUsageAccounting: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["usage"]; ok {
		t.Fatal("Xiaomi request received OpenRouter usage extension")
	}
	if _, ok := body["user"]; ok {
		t.Fatal("Xiaomi request received OpenRouter user attribution")
	}
	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

func TestApplyUsageAccounting_AtlasCloudUsesOpenAICompatibleContract(t *testing.T) {
	req := makePostRequest(`{"model":"hy3","stream":true,"messages":[]}`)

	if err := applyUsageAccounting(req, "atlascloud", "https://api.atlascloud.ai/v1", "ignored-user"); err != nil {
		t.Fatalf("applyUsageAccounting: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["usage"]; ok {
		t.Fatal("Atlas Cloud request received OpenRouter usage extension")
	}
	if _, ok := body["user"]; ok {
		t.Fatal("Atlas Cloud request received OpenRouter user attribution")
	}
	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

func TestApplyUsageAccounting_NovitaUsesOpenAICompatibleContract(t *testing.T) {
	req := makePostRequest(`{"model":"inclusionai/ling-3.0-flash","stream":true,"messages":[]}`)

	if err := applyUsageAccounting(req, "novita", "https://api.novita.ai/openai/v1", "ignored-user"); err != nil {
		t.Fatalf("applyUsageAccounting: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["usage"]; ok {
		t.Fatal("Novita request received OpenRouter usage extension")
	}
	if _, ok := body["user"]; ok {
		t.Fatal("Novita request received OpenRouter user attribution")
	}
	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

func TestApplyUsageAccounting_EngyUsesOpenAICompatibleContract(t *testing.T) {
	req := makePostRequest(`{"model":"glm-5.2","stream":true,"messages":[]}`)

	if err := applyUsageAccounting(req, "engy", "https://api.engy.ai/v1", "ignored-user"); err != nil {
		t.Fatalf("applyUsageAccounting: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["usage"]; ok {
		t.Fatal("Engy request received OpenRouter usage extension")
	}
	if _, ok := body["user"]; ok {
		t.Fatal("Engy request received OpenRouter user attribution")
	}
	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

func TestEnsureOpenAICompatibleUsage_XiaomiNonStreamingPreservesBody(t *testing.T) {
	req := makePostRequest(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`)

	if err := EnsureOpenAICompatibleUsage(req); err != nil {
		t.Fatalf("EnsureOpenAICompatibleUsage: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["stream_options"]; ok {
		t.Fatal("stream_options should be absent for non-streaming request")
	}
	if string(body["model"]) != `"mimo-v2.5-pro"` {
		t.Fatalf("model = %s, want mimo-v2.5-pro", body["model"])
	}
}

func TestEnsureOpenAICompatibleUsage_XiaomiReplacesNullStreamOptions(t *testing.T) {
	req := makePostRequest(`{"model":"mimo-v2.5-pro","stream":true,"stream_options":null,"messages":[]}`)

	if err := EnsureOpenAICompatibleUsage(req); err != nil {
		t.Fatalf("EnsureOpenAICompatibleUsage: %v", err)
	}

	body := decodeBody(t, req)
	so := map[string]json.RawMessage{}
	if err := json.Unmarshal(body["stream_options"], &so); err != nil {
		t.Fatalf("stream_options not an object: %s", body["stream_options"])
	}
	if string(so["include_usage"]) != "true" {
		t.Fatalf("include_usage = %s, want true", so["include_usage"])
	}
}

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
