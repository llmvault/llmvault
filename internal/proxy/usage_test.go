package proxy

import (
	"math"
	"testing"
)

// --- Non-streaming tests ---

func TestParseUsageNonStreaming_OpenAI(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-123",
		"choices": [{"message": {"content": "hello"}}],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 50,
			"prompt_tokens_details": {"cached_tokens": 20},
			"completion_tokens_details": {"reasoning_tokens": 10}
		}
	}`)

	u := ParseUsageNonStreaming("openai", body)
	assertUsage(t, u, 100, 50, 20, 10)
}

func TestParseUsageNonStreaming_OpenAI_NoDetails(t *testing.T) {
	body := []byte(`{
		"usage": {
			"prompt_tokens": 200,
			"completion_tokens": 75
		}
	}`)

	u := ParseUsageNonStreaming("openai", body)
	assertUsage(t, u, 200, 75, 0, 0)
}

func TestParseUsageNonStreaming_NovitaLing(t *testing.T) {
	// Sanitized from a live Novita inclusionai/ling-3.0-flash response.
	// Novita reports tokens but no request-level dollar cost.
	body := []byte(`{
		"id": "chatcmpl-sanitized",
		"model": "inclusionai/ling-3.0-flash",
		"choices": [{"message": {"content": "NOVITA_OK", "reasoning_content": "We need answer exactly."}}],
		"usage": {
			"prompt_tokens": 27,
			"completion_tokens": 36,
			"total_tokens": 63,
			"prompt_tokens_details": null,
			"completion_tokens_details": null
		}
	}`)

	u := ParseUsageNonStreaming("novita", body)
	assertUsage(t, u, 27, 36, 0, 0)
}

func TestParseUsageNonStreaming_NovitaReasoningIsCompletionBreakdown(t *testing.T) {
	// Sanitized from a live paid Novita deepseek/deepseek-v4-flash response.
	body := []byte(`{
		"model": "deepseek/deepseek-v4-flash",
		"usage": {
			"prompt_tokens": 9,
			"completion_tokens": 32,
			"total_tokens": 41,
			"completion_tokens_details": {"reasoning_tokens": 32}
		}
	}`)

	u := ParseUsageNonStreaming("novita", body)
	assertUsage(t, u, 9, 32, 0, 32)
}

func TestParseUsageNonStreaming_EngyProviderCharge(t *testing.T) {
	// Sanitized from a live Engy glm-5.2 response. Engy reports its actual
	// rounded charge in millionths of a USD alongside OpenAI token usage.
	body := []byte(`{
		"id": "chatcmpl-sanitized",
		"model": "glm-5.2",
		"usage": {
			"prompt_tokens": 17,
			"completion_tokens": 32,
			"total_tokens": 49,
			"prompt_tokens_details": {"cached_tokens": 0}
		},
		"x_engy": {
			"request_id": "request-sanitized",
			"miner": "miner-sanitized",
			"charged_micro": 60
		}
	}`)

	u := ParseUsageNonStreaming("engy", body)
	assertUsage(t, u, 17, 32, 0, 0)
	if math.Abs(u.ProviderCostUSD-0.000060) > 1e-12 {
		t.Fatalf("ProviderCostUSD = %.12f, want 0.000060", u.ProviderCostUSD)
	}
}

func TestParseUsageNonStreaming_Anthropic(t *testing.T) {
	body := []byte(`{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "hello"}],
		"usage": {
			"input_tokens": 150,
			"output_tokens": 80,
			"cache_creation_input_tokens": 30,
			"cache_read_input_tokens": 40
		}
	}`)

	u := ParseUsageNonStreaming("anthropic", body)
	assertUsage(t, u, 150, 80, 40, 0)
}

func TestParseUsageNonStreaming_Google(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hello"}]}}],
		"usageMetadata": {
			"promptTokenCount": 120,
			"candidatesTokenCount": 60,
			"cachedContentTokenCount": 25,
			"totalTokenCount": 180
		}
	}`)

	u := ParseUsageNonStreaming("google", body)
	assertUsage(t, u, 120, 60, 25, 0)
}

func TestParseUsageNonStreaming_UnknownProvider(t *testing.T) {
	// Unknown providers use OpenAI format as fallback
	body := []byte(`{
		"usage": {
			"prompt_tokens": 50,
			"completion_tokens": 25
		}
	}`)

	u := ParseUsageNonStreaming("together", body)
	assertUsage(t, u, 50, 25, 0, 0)
}

func TestParseUsageNonStreaming_MalformedJSON(t *testing.T) {
	body := []byte(`{broken json`)

	u := ParseUsageNonStreaming("openai", body)
	assertUsage(t, u, 0, 0, 0, 0)
}

func TestParseUsageNonStreaming_NoUsageField(t *testing.T) {
	body := []byte(`{"id":"123","choices":[]}`)

	u := ParseUsageNonStreaming("openai", body)
	assertUsage(t, u, 0, 0, 0, 0)
}

func TestParseUsageNonStreaming_EmptyBody(t *testing.T) {
	u := ParseUsageNonStreaming("openai", []byte{})
	assertUsage(t, u, 0, 0, 0, 0)
}

// --- Streaming tests ---

func TestParseUsageStreaming_OpenAIFinalChunk(t *testing.T) {
	events := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50}}\n\ndata: [DONE]\n\n")

	u := ParseUsageStreaming("openai", events)
	assertUsage(t, u, 100, 50, 0, 0)
}

func TestParseUsageStreaming_OpenAIWithReasoning(t *testing.T) {
	events := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":200,\"completion_tokens\":100,\"completion_tokens_details\":{\"reasoning_tokens\":30}}}\n\ndata: [DONE]\n\n")

	u := ParseUsageStreaming("openai", events)
	assertUsage(t, u, 200, 100, 0, 30)
}

func TestParseUsageStreaming_AtlasCloudTopLevelReasoningAndCache(t *testing.T) {
	// Sanitized from a live Atlas Cloud tencent/hy3 stream after the stable
	// prompt prefix hit the provider cache.
	events := []byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Checking\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":6918,\"total_tokens\":6982,\"completion_tokens\":64,\"prompt_tokens_details\":{\"cached_tokens\":6912},\"reasoning_tokens\":64}}\n\ndata: [DONE]\n\n")

	u := ParseUsageStreaming("atlascloud", events)
	assertUsage(t, u, 6918, 64, 6912, 64)
}

func TestParseUsageStreaming_NovitaFinalUsageChunk(t *testing.T) {
	// Sanitized from a live Novita inclusionai/ling-3.0-flash stream.
	// The initial zero-valued usage object must not replace the final summary.
	events := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"NOVITA_OK\"}}],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0,\"total_tokens\":0}}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":27,\"completion_tokens\":16,\"total_tokens\":43}}\n\ndata: [DONE]\n\n")

	u := ParseUsageStreaming("novita", events)
	assertUsage(t, u, 27, 16, 0, 0)
}

func TestParseUsageStreaming_EngyMergesChargeAndUsageChunks(t *testing.T) {
	// Sanitized from a live Engy qwen3.6-35b-a3b stream. The finish chunk
	// carries the charge and the following summary chunk carries token usage.
	events := []byte("data: {\"id\":\"chatcmpl-sanitized\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"x_engy\":{\"request_id\":\"request-sanitized\",\"miner\":\"miner-sanitized\",\"charged_micro\":10}}\n\ndata: {\"id\":\"chatcmpl-sanitized\",\"choices\":[],\"usage\":{\"prompt_tokens\":15,\"completion_tokens\":32,\"total_tokens\":47,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\ndata: [DONE]\n\n")

	u := ParseUsageStreaming("engy", events)
	assertUsage(t, u, 15, 32, 0, 0)
	if math.Abs(u.ProviderCostUSD-0.000010) > 1e-12 {
		t.Fatalf("ProviderCostUSD = %.12f, want 0.000010", u.ProviderCostUSD)
	}
}

func TestParseUsageStreaming_AnthropicMessageDelta(t *testing.T) {
	events := []byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":150,\"output_tokens\":0}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hi\"}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":150,\"output_tokens\":80,\"cache_read_input_tokens\":40}}\n\ndata: {\"type\":\"message_stop\"}\n\n")

	u := ParseUsageStreaming("anthropic", events)
	assertUsage(t, u, 150, 80, 40, 0)
}

func TestParseUsageStreaming_GoogleFormat(t *testing.T) {
	events := []byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]}}]}\n\ndata: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":100,\"candidatesTokenCount\":50,\"cachedContentTokenCount\":10}}\n\n")

	u := ParseUsageStreaming("google", events)
	assertUsage(t, u, 100, 50, 10, 0)
}

func TestParseUsageStreaming_NoUsageEvents(t *testing.T) {
	events := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")

	u := ParseUsageStreaming("openai", events)
	assertUsage(t, u, 0, 0, 0, 0)
}

func TestParseUsageStreaming_EmptyInput(t *testing.T) {
	u := ParseUsageStreaming("openai", []byte{})
	assertUsage(t, u, 0, 0, 0, 0)
}

// --- ParseStreamingChunk tests ---

func TestParseStreamingChunk_OpenAI(t *testing.T) {
	data := []byte(`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	u := ParseStreamingChunk("openai", data)
	assertUsage(t, u, 100, 50, 0, 0)
}

func TestParseStreamingChunk_Anthropic_MessageStart(t *testing.T) {
	data := []byte(`{"type":"message_start","message":{"usage":{"input_tokens":150,"output_tokens":0}}}`)
	u := ParseStreamingChunk("anthropic", data)
	assertUsage(t, u, 150, 0, 0, 0)
}

func TestParseStreamingChunk_Anthropic_MessageDelta(t *testing.T) {
	data := []byte(`{"type":"message_delta","usage":{"input_tokens":150,"output_tokens":80,"cache_read_input_tokens":40}}`)
	u := ParseStreamingChunk("anthropic", data)
	assertUsage(t, u, 150, 80, 40, 0)
}

func TestParseStreamingChunk_Google(t *testing.T) {
	data := []byte(`{"usageMetadata":{"promptTokenCount":120,"candidatesTokenCount":60}}`)
	u := ParseStreamingChunk("google", data)
	assertUsage(t, u, 120, 60, 0, 0)
}

func TestParseStreamingChunk_MalformedJSON(t *testing.T) {
	data := []byte(`{broken`)
	u := ParseStreamingChunk("openai", data)
	assertUsage(t, u, 0, 0, 0, 0)
}

// --- Provider detection tests ---

func TestIsAnthropicProvider(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"anthropic", true},
		{"anthropic-vertex", true},
		{"openai", false},
		{"google", false},
	}
	for _, tt := range tests {
		if got := isAnthropicProvider(tt.id); got != tt.want {
			t.Errorf("isAnthropicProvider(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestIsGoogleProvider(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"google", true},
		{"google-vertex", true},
		{"google-ai-studio", true},
		{"openai", false},
		{"anthropic", false},
	}
	for _, tt := range tests {
		if got := isGoogleProvider(tt.id); got != tt.want {
			t.Errorf("isGoogleProvider(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func assertUsage(t *testing.T, u UsageData, input, output, cached, reasoning int) {
	t.Helper()
	if u.InputTokens != input {
		t.Errorf("InputTokens = %d, want %d", u.InputTokens, input)
	}
	if u.OutputTokens != output {
		t.Errorf("OutputTokens = %d, want %d", u.OutputTokens, output)
	}
	if u.CachedTokens != cached {
		t.Errorf("CachedTokens = %d, want %d", u.CachedTokens, cached)
	}
	if u.ReasoningTokens != reasoning {
		t.Errorf("ReasoningTokens = %d, want %d", u.ReasoningTokens, reasoning)
	}
}
