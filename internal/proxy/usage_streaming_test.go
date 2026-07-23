package proxy

import (
	"math"
	"testing"
)

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
	events := []byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Checking\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":6918,\"total_tokens\":6982,\"completion_tokens\":64,\"prompt_tokens_details\":{\"cached_tokens\":6912},\"reasoning_tokens\":64}}\n\ndata: [DONE]\n\n")
	u := ParseUsageStreaming("atlascloud", events)
	assertUsage(t, u, 6918, 64, 6912, 64)
}

func TestParseUsageStreaming_NovitaFinalUsageChunk(t *testing.T) {
	events := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"NOVITA_OK\"}}],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0,\"total_tokens\":0}}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":27,\"completion_tokens\":16,\"total_tokens\":43}}\n\ndata: [DONE]\n\n")
	u := ParseUsageStreaming("novita", events)
	assertUsage(t, u, 27, 16, 0, 0)
}

func TestParseUsageStreaming_EngyMergesChargeAndUsageChunks(t *testing.T) {
	events := []byte("data: {\"id\":\"chatcmpl-sanitized\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"x_engy\":{\"request_id\":\"request-sanitized\",\"miner\":\"miner-sanitized\",\"charged_micro\":10}}\n\ndata: {\"id\":\"chatcmpl-sanitized\",\"choices\":[],\"usage\":{\"prompt_tokens\":15,\"completion_tokens\":32,\"total_tokens\":47,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\ndata: [DONE]\n\n")
	u := ParseUsageStreaming("engy", events)
	assertUsage(t, u, 15, 32, 0, 0)
	if math.Abs(u.ProviderCostUSD-0.000010) > 1e-12 {
		t.Fatalf("ProviderCostUSD = %.12f, want 0.000010", u.ProviderCostUSD)
	}
}

func TestParseUsageStreaming_TogetherFinalUsageChunk(t *testing.T) {
	events := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"The\"}}],\"usage\":null}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":32,\"total_tokens\":52,\"prompt_tokens_details\":{\"cached_tokens\":0},\"completion_tokens_details\":{\"reasoning_tokens\":28}}}\n\ndata: [DONE]\n\n")
	u := ParseUsageStreaming("together", events)
	assertUsage(t, u, 20, 32, 0, 28)
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

func TestParseStreamingChunk_OpenAI(t *testing.T) {
	u := ParseStreamingChunk("openai", []byte(`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50}}`))
	assertUsage(t, u, 100, 50, 0, 0)
}

func TestParseStreamingChunk_Anthropic_MessageStart(t *testing.T) {
	u := ParseStreamingChunk("anthropic", []byte(`{"type":"message_start","message":{"usage":{"input_tokens":150,"output_tokens":0}}}`))
	assertUsage(t, u, 150, 0, 0, 0)
}

func TestParseStreamingChunk_Anthropic_MessageDelta(t *testing.T) {
	u := ParseStreamingChunk("anthropic", []byte(`{"type":"message_delta","usage":{"input_tokens":150,"output_tokens":80,"cache_read_input_tokens":40}}`))
	assertUsage(t, u, 150, 80, 40, 0)
}

func TestParseStreamingChunk_Google(t *testing.T) {
	u := ParseStreamingChunk("google", []byte(`{"usageMetadata":{"promptTokenCount":120,"candidatesTokenCount":60}}`))
	assertUsage(t, u, 120, 60, 0, 0)
}

func TestParseStreamingChunk_MalformedJSON(t *testing.T) {
	u := ParseStreamingChunk("openai", []byte(`{broken`))
	assertUsage(t, u, 0, 0, 0, 0)
}

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
