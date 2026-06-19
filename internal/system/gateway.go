package system

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

type Gateway interface {
	Complete(ctx context.Context, call ForwardCall) (*CompletionResult, error)
	Stream(ctx context.Context, call ForwardCall, w http.ResponseWriter) (*CompletionResult, error)
}

// GenkitGateway runs server-side model calls through Genkit's provider
// abstraction. Each call carries the already-resolved Hivy credential and
// upstream model id; the gateway only translates that into Genkit messages and
// config.
type GenkitGateway struct {
	HTTPClient *http.Client
}

func NewGenkitGateway(c *http.Client) *GenkitGateway {
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	return &GenkitGateway{HTTPClient: c}
}

func (g *GenkitGateway) Complete(ctx context.Context, call ForwardCall) (*CompletionResult, error) {
	if call.Stream {
		return nil, errors.New("GenkitGateway.Complete: call.Stream must be false")
	}
	kit, modelName, err := g.init(ctx, call)
	if err != nil {
		return nil, err
	}
	opts := g.options(modelName, call.Request)
	resp, err := genkit.Generate(ctx, kit, opts...)
	if err != nil {
		return nil, normalizeGenkitError(err)
	}
	return completionFromResponse(resp, call.Request.Model), nil
}

func (g *GenkitGateway) Stream(ctx context.Context, call ForwardCall, w http.ResponseWriter) (*CompletionResult, error) {
	if !call.Stream {
		return nil, errors.New("GenkitGateway.Stream: call.Stream must be true")
	}
	kit, modelName, err := g.init(ctx, call)
	if err != nil {
		return nil, err
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	rc := http.NewResponseController(w)

	opts := g.options(modelName, call.Request)
	var text strings.Builder
	var final *ai.ModelResponse
	for value, err := range genkit.GenerateStream(ctx, kit, opts...) {
		if err != nil {
			return nil, normalizeGenkitError(err)
		}
		if value.Done {
			final = value.Response
			break
		}
		if value.Chunk == nil {
			continue
		}
		delta := value.Chunk.Text()
		if delta == "" {
			continue
		}
		text.WriteString(delta)
		if err := writeSSE(w, sseDelta{Delta: delta}); err != nil {
			return nil, err
		}
		_ = rc.Flush()
	}

	usage := Usage{}
	modelID := call.Request.Model
	if final != nil {
		usage = usageFromGenkit(final.Usage)
		modelID = modelFromGenkit(final, modelID)
	}
	if err := writeSSE(w, sseDone{Done: true, Usage: usage}); err != nil {
		return nil, err
	}
	_ = rc.Flush()

	return &CompletionResult{
		Text:  text.String(),
		Model: modelID,
		Usage: usage,
	}, nil
}

func (g *GenkitGateway) init(ctx context.Context, call ForwardCall) (*genkit.Genkit, string, error) {
	if call.Request == nil {
		return nil, "", errors.New("genkit gateway: request is required")
	}
	if strings.TrimSpace(call.Request.Model) == "" {
		return nil, "", errors.New("genkit gateway: model is required")
	}
	if strings.TrimSpace(call.APIKey) == "" {
		return nil, "", errors.New("genkit gateway: api key is required")
	}
	if strings.TrimSpace(call.BaseURL) == "" {
		return nil, "", errors.New("genkit gateway: base url is required")
	}

	provider := strings.TrimSpace(call.ProviderID)
	if provider == "" {
		provider = "openai"
	}

	opts := []option.RequestOption{option.WithAPIKey(call.APIKey)}
	opts = append(opts, option.WithBaseURL(strings.TrimRight(call.BaseURL, "/")))
	if g.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(g.HTTPClient))
	}

	plugin := &compat_oai.OpenAICompatible{
		Provider: provider,
		Opts:     opts,
	}
	kit := genkit.Init(ctx, genkit.WithPlugins(plugin))
	return kit, provider + "/" + call.Request.Model, nil
}

func (g *GenkitGateway) options(modelName string, req *LLMRequest) []ai.GenerateOption {
	opts := []ai.GenerateOption{
		ai.WithModelName(modelName),
		ai.WithMessages(messagesFromRequest(req)...),
	}
	if cfg := configFromRequest(req); cfg != nil {
		opts = append(opts, ai.WithConfig(cfg))
	}
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		opts = append(opts, ai.WithOutputFormat(ai.OutputFormatJSON))
	}
	return opts
}

func messagesFromRequest(req *LLMRequest) []*ai.Message {
	out := make([]*ai.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := ai.RoleUser
		switch msg.Role {
		case "system":
			role = ai.RoleSystem
		case "assistant", "model":
			role = ai.RoleModel
		case "tool":
			role = ai.RoleTool
		}
		parts := make([]*ai.Part, 0, max(1, len(msg.Parts)))
		if len(msg.Parts) == 0 {
			parts = append(parts, ai.NewTextPart(msg.Content))
		} else {
			for _, part := range msg.Parts {
				switch part.Kind {
				case LLMPartMedia:
					parts = append(parts, ai.NewMediaPart(part.ContentType, part.Text))
				default:
					parts = append(parts, ai.NewTextPart(part.Text))
				}
			}
		}
		out = append(out, ai.NewMessage(role, nil, parts...))
	}
	return out
}

func configFromRequest(req *LLMRequest) *openai.ChatCompletionNewParams {
	if req == nil {
		return nil
	}
	var cfg openai.ChatCompletionNewParams
	hasConfig := false
	if req.MaxTokens > 0 {
		cfg.MaxTokens = param.NewOpt(int64(req.MaxTokens))
		hasConfig = true
	}
	if req.Temperature != nil {
		cfg.Temperature = param.NewOpt(float64(*req.Temperature))
		hasConfig = true
	}
	if req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
		cfg.StreamOptions.IncludeUsage = param.NewOpt(true)
		hasConfig = true
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		cfg.ReasoningEffort = shared.ReasoningEffort(req.Reasoning.Effort)
		hasConfig = true
	}
	if !hasConfig {
		return nil
	}
	return &cfg
}

func completionFromResponse(resp *ai.ModelResponse, fallbackModel string) *CompletionResult {
	if resp == nil {
		return &CompletionResult{Model: fallbackModel}
	}
	return &CompletionResult{
		Text:  resp.Text(),
		Model: modelFromGenkit(resp, fallbackModel),
		Usage: usageFromGenkit(resp.Usage),
	}
}

func usageFromGenkit(u *ai.GenerationUsage) Usage {
	if u == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		CachedTokens:    u.CachedContentTokens,
		ReasoningTokens: u.ThoughtsTokens,
	}
}

func modelFromGenkit(resp *ai.ModelResponse, fallback string) string {
	if resp == nil || resp.Custom == nil {
		return fallback
	}
	custom, ok := resp.Custom.(map[string]any)
	if !ok {
		return fallback
	}
	if model, ok := custom["model"].(string); ok && model != "" {
		return model
	}
	return fallback
}

func normalizeGenkitError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return &UpstreamError{
			StatusCode: apiErr.StatusCode,
			Body:       apiErr.Error(),
		}
	}
	return fmt.Errorf("genkit generate: %w", err)
}
