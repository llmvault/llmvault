package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/billing"
)

const CostPerHourUSD = 0.22

func CostUSD(durationSeconds float64) float64 {
	cost := durationSeconds / 3600 * CostPerHourUSD
	if cost <= 0 {
		return billing.CreditUSDValue
	}
	return cost
}

type Request struct {
	APIKey       []byte
	BaseURL      string
	ModelID      string
	Audio        []byte
	Filename     string
	ContentType  string
	LanguageCode string
}

type Result struct {
	Text            string
	LanguageCode    string
	DurationSeconds float64
}

type Transcriber interface {
	Transcribe(ctx context.Context, req Request) (Result, error)
}

type ElevenLabsTranscriber struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewElevenLabsTranscriber(httpClient *http.Client, timeout time.Duration) *ElevenLabsTranscriber {
	return &ElevenLabsTranscriber{httpClient: httpClient, timeout: timeout}
}

func (t *ElevenLabsTranscriber) Transcribe(ctx context.Context, req Request) (Result, error) {
	if len(req.APIKey) == 0 {
		return Result{}, fmt.Errorf("elevenlabs api key is required")
	}
	if len(req.Audio) == 0 {
		return Result{}, fmt.Errorf("audio is required")
	}
	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		modelID = "scribe_v2"
	}

	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	endpoint, err := elevenLabsSpeechToTextURL(req.BaseURL)
	if err != nil {
		return Result{}, err
	}
	body, contentType, err := elevenLabsTranscriptionBody(req, modelID)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return Result{}, fmt.Errorf("create elevenlabs request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("xi-api-key", string(req.APIKey))

	client := t.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("elevenlabs transcribe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, elevenLabsStatusError(resp)
	}

	var decoded elevenLabsTranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Result{}, fmt.Errorf("elevenlabs transcribe: decode response: %w", err)
	}
	return Result{
		Text:            decoded.Text,
		LanguageCode:    decoded.LanguageCode,
		DurationSeconds: decoded.durationSeconds(),
	}, nil
}

type elevenLabsTranscriptionResponse struct {
	Text              string  `json:"text"`
	LanguageCode      string  `json:"language_code"`
	AudioDurationSecs float64 `json:"audio_duration_secs"`
	Words             []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	} `json:"words,omitempty"`
}

func (r elevenLabsTranscriptionResponse) durationSeconds() float64 {
	if r.AudioDurationSecs > 0 {
		return r.AudioDurationSecs
	}
	var maxEnd float64
	for _, word := range r.Words {
		if word.End > maxEnd {
			maxEnd = word.End
		}
	}
	if maxEnd < 0 {
		return 0
	}
	return maxEnd
}

func elevenLabsSpeechToTextURL(baseURL string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if raw == "" {
		raw = "https://api.elevenlabs.io"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid elevenlabs base_url")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/v1") {
		parsed.Path = basePath + "/speech-to-text"
	} else {
		parsed.Path = basePath + "/v1/speech-to-text"
	}
	return parsed.String(), nil
}

func elevenLabsTranscriptionBody(req Request, modelID string) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("model_id", modelID); err != nil {
		return nil, "", fmt.Errorf("build elevenlabs request: %w", err)
	}
	if languageCode := strings.TrimSpace(req.LanguageCode); languageCode != "" {
		if err := writer.WriteField("language_code", languageCode); err != nil {
			return nil, "", fmt.Errorf("build elevenlabs request: %w", err)
		}
	}
	if err := writeElevenLabsAudioPart(writer, req); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("build elevenlabs request: %w", err)
	}
	return body, writer.FormDataContentType(), nil
}

func writeElevenLabsAudioPart(writer *multipart.Writer, req Request) error {
	filename, contentType := elevenLabsAudioPartMetadata(req)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", multipart.FileContentDisposition("file", filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("build elevenlabs request: %w", err)
	}
	if _, err := part.Write(req.Audio); err != nil {
		return fmt.Errorf("build elevenlabs request: %w", err)
	}
	return nil
}

func elevenLabsAudioPartMetadata(req Request) (string, string) {
	filename := strings.TrimSpace(req.Filename)
	filename = path.Base(strings.ReplaceAll(filename, "\\", "/"))
	if filename == "" || filename == "." || filename == "/" {
		filename = "audio.webm"
	}

	contentType := strings.TrimSpace(req.ContentType)
	if semicolon := strings.Index(contentType, ";"); semicolon >= 0 {
		contentType = strings.TrimSpace(contentType[:semicolon])
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return filename, contentType
}

func elevenLabsStatusError(resp *http.Response) error {
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := strings.TrimSpace(string(responseBody))
	if body == "" {
		return fmt.Errorf("elevenlabs transcribe: status %d", resp.StatusCode)
	}
	return fmt.Errorf("elevenlabs transcribe: status %d: %s", resp.StatusCode, body)
}
