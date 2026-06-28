package hivy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	openai "github.com/sashabaranov/go-openai"
	"github.com/usehivy/hivy/internal/providerheaders"
)

// OpenAICompletionClient implements CompletionClient for OpenAI and all
// OpenAI-compatible providers (Fireworks, OpenRouter, DeepSeek, Groq,
// Together, xAI, Mistral, Cohere). The BaseURL on the credential determines
// which upstream receives the request.
type OpenAICompletionClient struct {
	client *openai.Client
}

// NewOpenAICompletionClient creates a client pointed at the given base URL.
// For OpenAI proper, use "https://api.openai.com/v1". For other providers,
// use their OpenAI-compatible endpoint.
func NewOpenAICompletionClient(baseURL, apiKey string) *OpenAICompletionClient {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if providerheaders.IsOpenRouter("", cfg.BaseURL) {
		cfg.HTTPClient = openRouterHeaderDoer{inner: cfg.HTTPClient}
	}
	return &OpenAICompletionClient{client: openai.NewClientWithConfig(cfg)}
}

type openRouterHeaderDoer struct {
	inner openai.HTTPDoer
}

func (d openRouterHeaderDoer) Do(req *http.Request) (*http.Response, error) {
	providerheaders.ApplyOpenRouter(req)
	return d.inner.Do(req)
}

func (c *OpenAICompletionClient) ChatCompletion(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for index, message := range req.Messages {
		msg := openai.ChatCompletionMessage{
			Role:    message.Role,
			Content: message.Content,
		}
		if message.ToolCallID != "" {
			msg.ToolCallID = message.ToolCallID
		}
		if message.Name != "" {
			msg.Name = message.Name
		}
		for _, toolCall := range message.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
				ID:   toolCall.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			})
		}
		messages[index] = msg
	}

	tools := make([]openai.Tool, len(req.Tools))
	for index, tool := range req.Tools {
		tools[index] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}
	}

	oaiReq := openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: messages,
		Tools:    tools,
	}
	if req.MaxTokens > 0 {
		oaiReq.MaxTokens = req.MaxTokens
	}
	if req.ResponseFormat != nil {
		responseFormat, err := openAIResponseFormat(req.ResponseFormat)
		if err != nil {
			return nil, err
		}
		oaiReq.ResponseFormat = responseFormat
	}
	if req.ToolChoice == "required" {
		oaiReq.ToolChoice = openai.ToolChoice{
			Type: openai.ToolTypeFunction,
		}
	}

	resp, err := c.client.CreateChatCompletion(ctx, oaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai completion: empty choices")
	}

	choice := resp.Choices[0]
	result := &CompletionResponse{
		Message: Message{
			Role:    choice.Message.Role,
			Content: choice.Message.Content,
		},
	}
	for _, toolCall := range choice.Message.ToolCalls {
		result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		})
	}

	return result, nil
}

func openAIResponseFormat(format *ResponseFormat) (*openai.ChatCompletionResponseFormat, error) {
	if format == nil {
		return nil, nil
	}
	switch format.Type {
	case ResponseFormatJSONSchema:
		if format.JSONSchema == nil {
			return nil, fmt.Errorf("openai completion: missing json schema response format")
		}
		if !json.Valid(format.JSONSchema.Schema) {
			return nil, fmt.Errorf("openai completion: invalid json schema response format")
		}
		return &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:        format.JSONSchema.Name,
				Description: format.JSONSchema.Description,
				Schema:      format.JSONSchema.Schema,
				Strict:      format.JSONSchema.Strict,
			},
		}, nil
	default:
		return nil, fmt.Errorf("openai completion: unsupported response format %q", format.Type)
	}
}
