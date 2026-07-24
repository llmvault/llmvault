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

func TestParseUsageNonStreaming_TheGridEstimatedCost(t *testing.T) {
	body := []byte(`{
		"model": "openai/gpt-oss-120b",
		"usage": {
			"prompt_tokens": 60,
			"completion_tokens": 16,
			"total_tokens": 76,
			"estimated_cost": 0.00000494,
			"completion_tokens_details": {"reasoning_tokens": 13}
		}
	}`)

	u := ParseUsageNonStreaming("thegrid", body)
	assertUsage(t, u, 60, 16, 0, 13)
	if math.Abs(u.ProviderCostUSD-0.00000494) > 1e-12 {
		t.Fatalf("ProviderCostUSD = %.12f, want 0.00000494", u.ProviderCostUSD)
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

func TestParseUsageNonStreaming_TogetherTopLevelDetails(t *testing.T) {
	// Together documents cached_tokens and reasoning_tokens as optional
	// Together-specific usage fields that can be returned at the top level.
	body := []byte(`{
		"usage": {
			"prompt_tokens": 200,
			"completion_tokens": 75,
			"cached_tokens": 160,
			"reasoning_tokens": 50
		}
	}`)

	u := ParseUsageNonStreaming("together", body)
	assertUsage(t, u, 200, 75, 160, 50)
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
