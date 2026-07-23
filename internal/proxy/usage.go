package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
)

// UsageData holds normalized token usage extracted from an LLM provider response.
type UsageData struct {
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	ReasoningTokens int
	ProviderCostUSD float64
}

// ParseUsageNonStreaming extracts token usage from a complete (non-streaming)
// JSON response body. It inspects the provider ID to use the correct format.
func ParseUsageNonStreaming(providerID string, body []byte) UsageData {
	switch {
	case isAnthropicProvider(providerID):
		return parseAnthropicUsage(body)
	case isGoogleProvider(providerID):
		return parseGoogleUsage(body)
	default:
		// OpenAI format is the most common default (also used by many proxies)
		return parseOpenAIUsage(providerID, body)
	}
}

// ParseUsageStreaming extracts token usage from SSE streaming events.
// It scans all events and returns the usage from the final/summary event.
func ParseUsageStreaming(providerID string, events []byte) UsageData {
	var usage UsageData
	scanner := bufio.NewScanner(bytes.NewReader(events))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		u := parseStreamingChunk(providerID, []byte(data))
		mergeUsage(&usage, u)
	}
	return usage
}

// ParseStreamingChunk extracts usage from a single SSE data chunk.
// Exported for use by the capture transport which processes chunks one at a time.
func ParseStreamingChunk(providerID string, data []byte) UsageData {
	return parseStreamingChunk(providerID, data)
}

func parseStreamingChunk(providerID string, data []byte) UsageData {
	switch {
	case isAnthropicProvider(providerID):
		return parseAnthropicStreamChunk(data)
	case isGoogleProvider(providerID):
		return parseGoogleUsage(data)
	default:
		return parseOpenAIStreamChunk(providerID, data)
	}
}

// --- OpenAI format ---

func parseOpenAIUsage(providerID string, body []byte) UsageData {
	var resp struct {
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			ReasoningTokens     int `json:"reasoning_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
		Engy *struct {
			ChargedMicro int64 `json:"charged_micro"`
		} `json:"x_engy"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return UsageData{}
	}
	u := UsageData{}
	if resp.Usage != nil {
		u.InputTokens = resp.Usage.PromptTokens
		u.OutputTokens = resp.Usage.CompletionTokens
		u.ReasoningTokens = resp.Usage.ReasoningTokens
		if resp.Usage.PromptTokensDetails != nil {
			u.CachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
		}
		if resp.Usage.CompletionTokensDetails != nil {
			u.ReasoningTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
		}
	}
	if providerID == "engy" && resp.Engy != nil && resp.Engy.ChargedMicro > 0 {
		u.ProviderCostUSD = float64(resp.Engy.ChargedMicro) / 1_000_000
	}
	return u
}

func parseOpenAIStreamChunk(providerID string, data []byte) UsageData {
	// OpenAI sends usage in the final chunk with usage field
	return parseOpenAIUsage(providerID, data)
}

func mergeUsage(current *UsageData, next UsageData) {
	if next.InputTokens > 0 || next.OutputTokens > 0 {
		current.InputTokens = next.InputTokens
		current.OutputTokens = next.OutputTokens
		current.CachedTokens = next.CachedTokens
		current.ReasoningTokens = next.ReasoningTokens
	}
	if next.ProviderCostUSD > 0 {
		current.ProviderCostUSD = next.ProviderCostUSD
	}
}

// --- Anthropic format ---

func parseAnthropicUsage(body []byte) UsageData {
	var resp struct {
		Usage *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return UsageData{}
	}
	return UsageData{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CachedTokens: resp.Usage.CacheReadInputTokens,
	}
}

func parseAnthropicStreamChunk(data []byte) UsageData {
	// Anthropic sends usage in message_delta and message_stop events
	var envelope struct {
		Type  string `json:"type"`
		Usage *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
		Message *struct {
			Usage *struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return UsageData{}
	}

	// message_start event contains usage in message.usage
	if envelope.Message != nil && envelope.Message.Usage != nil {
		u := envelope.Message.Usage
		return UsageData{
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			CachedTokens: u.CacheReadInputTokens,
		}
	}

	// message_delta event contains usage at top level
	if envelope.Usage != nil {
		return UsageData{
			InputTokens:  envelope.Usage.InputTokens,
			OutputTokens: envelope.Usage.OutputTokens,
			CachedTokens: envelope.Usage.CacheReadInputTokens,
		}
	}

	return UsageData{}
}

// --- Google format ---

func parseGoogleUsage(body []byte) UsageData {
	var resp struct {
		UsageMetadata *struct {
			PromptTokenCount        int `json:"promptTokenCount"`
			CandidatesTokenCount    int `json:"candidatesTokenCount"`
			CachedContentTokenCount int `json:"cachedContentTokenCount"`
			TotalTokenCount         int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.UsageMetadata == nil {
		return UsageData{}
	}
	return UsageData{
		InputTokens:  resp.UsageMetadata.PromptTokenCount,
		OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
		CachedTokens: resp.UsageMetadata.CachedContentTokenCount,
	}
}

// --- Provider detection ---

func isAnthropicProvider(id string) bool {
	return id == "anthropic" || strings.HasPrefix(id, "anthropic-")
}

func isGoogleProvider(id string) bool {
	return id == "google" || id == "google-vertex" || strings.HasPrefix(id, "google-")
}
