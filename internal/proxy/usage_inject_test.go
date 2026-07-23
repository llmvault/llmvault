package proxy

import (
	"encoding/json"
	"io"
	"net/http"
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

func TestApplyUsageAccounting_TogetherUsesOpenAICompatibleContract(t *testing.T) {
	req := makePostRequest(`{"model":"thinkingmachines/Inkling","stream":true,"messages":[]}`)

	if err := applyUsageAccounting(req, "together", "https://api.together.ai/v1", "ignored-user"); err != nil {
		t.Fatalf("applyUsageAccounting: %v", err)
	}

	body := decodeBody(t, req)
	if _, ok := body["usage"]; ok {
		t.Fatal("Together request received OpenRouter usage extension")
	}
	if _, ok := body["user"]; ok {
		t.Fatal("Together request received OpenRouter user attribution")
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
