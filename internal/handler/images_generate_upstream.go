package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

type imageReferenceInput struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

type openRouterImageRequest struct {
	Model           string                `json:"model"`
	Prompt          string                `json:"prompt"`
	N               int                   `json:"n,omitempty"`
	AspectRatio     string                `json:"aspect_ratio,omitempty"`
	InputReferences []imageReferenceInput `json:"input_references,omitempty"`
}

type openRouterImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		Base64  string `json:"base64,omitempty"`
		URL     string `json:"url,omitempty"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

type generatedImageBytes struct {
	Data []byte
}

func (h *UploadsHandler) callOpenRouterImages(ctx context.Context, cred *model.Credential, apiKey, upstreamModel, prompt, aspectRatio string, count int, references []imageReferenceInput) ([]generatedImageBytes, error) {
	payload := openRouterImageRequest{
		Model:           upstreamModel,
		Prompt:          prompt,
		N:               count,
		AspectRatio:     aspectRatio,
		InputReferences: references,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint, err := openRouterImagesEndpoint(cred.BaseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeaderValue(cred.AuthScheme, apiKey))

	client := h.imageHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post OpenRouter images: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenRouter images status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded openRouterImageResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode OpenRouter image response: %w", err)
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("OpenRouter image error: %s", decoded.Error.Message)
	}
	if len(decoded.Data) == 0 {
		return nil, fmt.Errorf("OpenRouter returned no images")
	}

	out := make([]generatedImageBytes, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		value := strings.TrimSpace(item.B64JSON)
		if value == "" {
			value = strings.TrimSpace(item.Base64)
		}
		if value != "" {
			data, err := decodeImageBase64(value)
			if err != nil {
				return nil, err
			}
			out = append(out, generatedImageBytes{Data: data})
			continue
		}
		if strings.TrimSpace(item.URL) != "" {
			data, err := h.downloadGeneratedImage(ctx, item.URL)
			if err != nil {
				return nil, err
			}
			out = append(out, generatedImageBytes{Data: data})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("OpenRouter returned no decodable images")
	}
	return out, nil
}

func openRouterImagesEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("OpenRouter base URL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse OpenRouter base URL: %w", err)
	}
	if strings.HasSuffix(u.Path, "/images") {
		return u.String(), nil
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/images"
	return u.String(), nil
}

func authHeaderValue(authScheme, apiKey string) string {
	if strings.EqualFold(strings.TrimSpace(authScheme), "bearer") || strings.TrimSpace(authScheme) == "" {
		return "Bearer " + apiKey
	}
	return strings.TrimSpace(authScheme) + " " + apiKey
}

func decodeImageBase64(value string) ([]byte, error) {
	if strings.HasPrefix(value, "data:") {
		comma := strings.Index(value, ",")
		if comma < 0 {
			return nil, fmt.Errorf("invalid image data URL")
		}
		value = value[comma+1:]
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode generated image: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("generated image is empty")
	}
	return data, nil
}

func (h *UploadsHandler) downloadGeneratedImage(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build image download request: %w", err)
	}
	client := h.imageHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download generated image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download generated image status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read generated image: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("downloaded generated image is empty")
	}
	return data, nil
}
