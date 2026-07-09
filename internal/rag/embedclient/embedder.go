package embedclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sentrygo "github.com/getsentry/sentry-go"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/providerheaders"
)

type EmbedderConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dim        uint32
	Timeout    time.Duration
	MaxRetries int
}

type Embedder struct {
	cfg  EmbedderConfig
	http *http.Client
}

func NewEmbedder(cfg EmbedderConfig) *Embedder {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Embedder{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (e *Embedder) Dim() uint32 { return e.cfg.Dim }

func (e *Embedder) Model() string { return e.cfg.Model }

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message  string          `json:"message"`
		Type     string          `json:"type"`
		Code     json.RawMessage `json:"code"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
	} `json:"error,omitempty"`
}

func (e *Embedder) Embed(ctx context.Context, inputs []string) ([][]float32, int, error) {
	if len(inputs) == 0 {
		return nil, 0, nil
	}
	payload := map[string]any{
		"model": e.cfg.Model,
		"input": inputs,
	}
	if e.cfg.Dim > 0 {
		payload["dimensions"] = e.cfg.Dim
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("embed: marshal: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt <= e.cfg.MaxRetries; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
			e.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
		if providerheaders.IsOpenRouter("", e.cfg.BaseURL) {
			providerheaders.ApplyOpenRouter(req)
		}

		resp, err := e.http.Do(req)
		if err != nil {
			lastErr = err
			backoff(attempt)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			if len(respBody) == 0 {
				// Empty 200 would crash json.Unmarshal with a misleading
				// "unexpected end of JSON input" and permanently fail the
				// asynq task. Returning a normal error lets the existing
				// retry/backoff loop handle it.
				headers := safeResponseHeaders(resp.Header)
				if hub := sentrygo.GetHubFromContext(ctx); hub != nil {
					hub.AddBreadcrumb(&sentrygo.Breadcrumb{
						Type:     "http",
						Category: "embed.empty_body",
						Message:  fmt.Sprintf("embed upstream empty body: status=%d model=%s", resp.StatusCode, e.cfg.Model),
						Level:    sentrygo.LevelWarning,
						Data: map[string]any{
							"status":         resp.StatusCode,
							"model":          e.cfg.Model,
							"content_length": resp.ContentLength,
							"headers":        headers,
						},
					}, nil)
				}
				logging.FromContext(ctx).WarnContext(ctx, "embed upstream returned empty body",
					"status", resp.StatusCode,
					"model", e.cfg.Model,
					"content_length", resp.ContentLength,
					"headers", headers,
				)
				lastErr = fmt.Errorf("embed: empty response body (status=%d)", resp.StatusCode)
				backoff(attempt)
				continue
			}
			var out embedResponse
			if err := json.Unmarshal(respBody, &out); err != nil {
				return nil, 0, fmt.Errorf("embed: decode: %w", err)
			}
			if out.Error != nil && out.Error.Message != "" {
				meta := ""
				if len(out.Error.Metadata) > 0 {
					meta = " metadata=" + string(out.Error.Metadata)
				}
				return nil, 0, fmt.Errorf("embed: upstream error: %s (type=%s code=%s)%s",
					out.Error.Message, out.Error.Type, string(out.Error.Code), meta)
			}
			if len(out.Data) != len(inputs) {
				preview := string(respBody)
				if len(preview) > 300 {
					preview = preview[:300]
				}
				return nil, 0, fmt.Errorf("embed: got %d vectors for %d inputs (body: %s)",
					len(out.Data), len(inputs), preview)
			}
			vectors := make([][]float32, len(out.Data))
			for i := range out.Data {
				vectors[i] = out.Data[i].Embedding
			}
			return vectors, out.Usage.TotalTokens, nil
		}
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		lastErr = fmt.Errorf("embed: %d: %s", resp.StatusCode, preview)
		// Don't retry on 4xx (apart from 429).
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			break
		}
		backoff(attempt)
	}
	return nil, 0, lastErr
}

func backoff(attempt int) {
	d := 250 * time.Millisecond
	for i := 0; i < attempt; i++ {
		d *= 2
	}
	if d > 4*time.Second {
		d = 4 * time.Second
	}
	time.Sleep(d)
}

// safeResponseHeaders returns resp.Header with values redacted for keys that
// look like they could carry credentials, matching the marker list used by
// internal/handler.isSensitivePayloadKey (lowercased, with - and _ treated
// as equivalent). Output is safe to ship to structured logs and Sentry
// breadcrumbs.
func safeResponseHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		normalized := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
		sensitive := false
		for _, marker := range []string{
			"authorization", "password", "secret", "token",
			"api_key", "apikey", "credential", "cookie", "set_cookie",
		} {
			if strings.Contains(normalized, marker) {
				sensitive = true
				break
			}
		}
		if sensitive {
			out[k] = "[redacted]"
			continue
		}
		out[k] = v[0]
	}
	return out
}
