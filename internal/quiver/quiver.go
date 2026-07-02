package quiver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultBaseURL = "https://api.quiver.ai"
	generatePath   = "/v1/svgs/generations"
	vectorizePath  = "/v1/svgs/vectorizations"
)

// GenerateSVG runs a text-to-svg generation call and returns the resulting SVGs.
func (c *Client) GenerateSVG(ctx context.Context, req GenerateRequest) (Result, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return Result{}, errors.New("quiver api key is required")
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return Result{}, errors.New("quiver prompt is required")
	}
	payload, err := buildGeneratePayload(req, prompt)
	if err != nil {
		return Result{}, err
	}
	return c.post(ctx, req.BaseURL, generatePath, req.AuthScheme, req.APIKey, payload)
}

// VectorizeSVG runs an image-to-svg vectorization call and returns the SVG.
func (c *Client) VectorizeSVG(ctx context.Context, req VectorizeRequest) (Result, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return Result{}, errors.New("quiver api key is required")
	}
	payload, err := buildVectorizePayload(req)
	if err != nil {
		return Result{}, err
	}
	return c.post(ctx, req.BaseURL, vectorizePath, req.AuthScheme, req.APIKey, payload)
}

func (c *Client) post(ctx context.Context, baseURL, path, authScheme, apiKey string, payload any) (Result, error) {
	endpoint, err := endpointURL(baseURL, path)
	if err != nil {
		return Result{}, err
	}

	if c != nil && c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("marshal quiver request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("create quiver request: %w", err)
	}
	httpReq.Header.Set("Authorization", authHeaderValue(authScheme, strings.TrimSpace(apiKey)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpClient := http.DefaultClient
	if c != nil && c.httpClient != nil {
		httpClient = c.httpClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("quiver svg generation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, quiverStatusError(resp)
	}
	return decodeResult(resp)
}

func endpointURL(baseURL, path string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = defaultBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid quiver base_url")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	return parsed.String(), nil
}

func authHeaderValue(authScheme, apiKey string) string {
	scheme := strings.TrimSpace(authScheme)
	if scheme == "" || strings.EqualFold(scheme, "bearer") {
		return "Bearer " + apiKey
	}
	return scheme + " " + apiKey
}

func decodeResult(resp *http.Response) (Result, error) {
	var decoded struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
		Credits int    `json:"credits"`
		Data    []struct {
			SVG      string `json:"svg"`
			MimeType string `json:"mime_type"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Result{}, fmt.Errorf("decode quiver response: %w", err)
	}
	svgs := make([]SVG, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		markup := item.SVG
		if strings.TrimSpace(markup) == "" {
			continue
		}
		mimeType := strings.TrimSpace(item.MimeType)
		if mimeType == "" {
			mimeType = "image/svg+xml"
		}
		svgs = append(svgs, SVG{Markup: markup, MimeType: mimeType})
	}
	return Result{
		SVGs:       svgs,
		ResponseID: decoded.ID,
		Credits:    decoded.Credits,
		RequestID:  resp.Header.Get("X-Request-ID"),
	}, nil
}

func quiverStatusError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	statusErr := &StatusError{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Request-ID"),
		RetryAfter: strings.TrimSpace(resp.Header.Get("Retry-After")),
	}
	if responseContentType(resp.Header.Get("Content-Type")) == "application/json" {
		var decoded struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
			Status    int    `json:"status"`
		}
		if err := json.Unmarshal(data, &decoded); err == nil {
			statusErr.Code = decoded.Code
			statusErr.Message = decoded.Message
			if strings.TrimSpace(decoded.RequestID) != "" {
				statusErr.RequestID = decoded.RequestID
			}
		}
	} else {
		statusErr.Body = strings.TrimSpace(string(data))
	}
	return statusErr
}

func responseContentType(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return mediaType
}
