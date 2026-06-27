package reve

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultBaseURL = "https://api.reve.com"
	createPath     = "/v1/image/create"
	maxInstruction = 4000
)

func (c *Client) GenerateImage(ctx context.Context, req GenerateRequest) (ImageResult, error) {
	payload, format, err := buildCreatePayload(req)
	if err != nil {
		return ImageResult{}, err
	}
	endpoint, err := createURL(req.BaseURL)
	if err != nil {
		return ImageResult{}, err
	}

	if c != nil && c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ImageResult{}, fmt.Errorf("marshal reve request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ImageResult{}, fmt.Errorf("create reve request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.APIKey))
	httpReq.Header.Set("Accept", string(format))
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := http.DefaultClient
	if c != nil && c.httpClient != nil {
		httpClient = c.httpClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return ImageResult{}, fmt.Errorf("reve image generation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ImageResult{}, reveStatusError(resp)
	}
	return decodeImageResult(resp, format)
}

func createURL(baseURL string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = defaultBaseURL
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid reve base_url")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + createPath
	return parsed.String(), nil
}

func validFormat(format ImageFormat) bool {
	switch format {
	case FormatPNG, FormatJPEG, FormatWEBP:
		return true
	default:
		return false
	}
}

func decodeImageResult(resp *http.Response, requested ImageFormat) (ImageResult, error) {
	contentType := responseContentType(resp.Header.Get("Content-Type"))
	if contentType == "application/json" {
		return decodeJSONResult(resp.Body)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ImageResult{}, fmt.Errorf("read reve image response: %w", err)
	}
	if contentType == "" {
		contentType = string(requested)
	}
	return ImageResult{
		Data:             data,
		ContentType:      contentType,
		Version:          resp.Header.Get("X-Reve-Version"),
		ContentViolation: headerBool(resp.Header.Get("X-Reve-Content-Violation")),
		RequestID:        resp.Header.Get("X-Reve-Request-Id"),
		CreditsUsed:      headerInt(resp.Header.Get("X-Reve-Credits-Used")),
		CreditsRemaining: headerInt(resp.Header.Get("X-Reve-Credits-Remaining")),
	}, nil
}

func decodeJSONResult(body io.Reader) (ImageResult, error) {
	var decoded struct {
		Image            string          `json:"image"`
		Data             string          `json:"data"`
		Layout           json.RawMessage `json:"layout"`
		Version          string          `json:"version"`
		ContentViolation bool            `json:"content_violation"`
		RequestID        string          `json:"request_id"`
		CreditsUsed      int             `json:"credits_used"`
		CreditsRemaining int             `json:"credits_remaining"`
	}
	if err := json.NewDecoder(body).Decode(&decoded); err != nil {
		return ImageResult{}, fmt.Errorf("decode reve response: %w", err)
	}
	data := []byte(nil)
	if decoded.Image != "" || decoded.Data != "" {
		var err error
		payload := decoded.Image
		if payload == "" {
			payload = decoded.Data
		}
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return ImageResult{}, fmt.Errorf("decode reve image: %w", err)
		}
	}
	return ImageResult{
		Data:             data,
		ContentType:      string(FormatPNG),
		Layout:           decoded.Layout,
		Version:          decoded.Version,
		ContentViolation: decoded.ContentViolation,
		RequestID:        decoded.RequestID,
		CreditsUsed:      decoded.CreditsUsed,
		CreditsRemaining: decoded.CreditsRemaining,
	}, nil
}

func reveStatusError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	statusErr := &StatusError{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Reve-Request-Id"),
		ErrorCode:  resp.Header.Get("X-Reve-Error-Code"),
	}
	if responseContentType(resp.Header.Get("Content-Type")) == "application/json" {
		var decoded struct {
			ErrorCode string         `json:"error_code"`
			Message   string         `json:"message"`
			Params    map[string]any `json:"params"`
		}
		if err := json.Unmarshal(data, &decoded); err == nil {
			if decoded.ErrorCode != "" {
				statusErr.ErrorCode = decoded.ErrorCode
			}
			statusErr.Message = decoded.Message
			statusErr.Params = decoded.Params
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

func headerBool(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func headerInt(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}
