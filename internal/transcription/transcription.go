package transcription

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	elevenlabs "github.com/plexusone/elevenlabs-go"
)

type Request struct {
	APIKey       []byte
	BaseURL      string
	ModelID      string
	Audio        []byte
	LanguageCode string
}

type Result struct {
	Text         string
	LanguageCode string
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

	opts := []elevenlabs.Option{
		elevenlabs.WithAPIKey(string(req.APIKey)),
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/"); baseURL != "" {
		opts = append(opts, elevenlabs.WithBaseURL(baseURL))
	}
	if t.httpClient != nil {
		opts = append(opts, elevenlabs.WithHTTPClient(t.httpClient))
	}
	if t.timeout > 0 {
		opts = append(opts, elevenlabs.WithTimeout(t.timeout))
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	client, err := elevenlabs.NewClient(opts...)
	if err != nil {
		return Result{}, fmt.Errorf("create elevenlabs client: %w", err)
	}
	resp, err := client.SpeechToText().Transcribe(ctx, &elevenlabs.TranscriptionRequest{
		FileContent:  base64.StdEncoding.EncodeToString(req.Audio),
		LanguageCode: strings.TrimSpace(req.LanguageCode),
		ModelID:      modelID,
	})
	if err != nil {
		return Result{}, fmt.Errorf("elevenlabs transcribe: %w", err)
	}
	if resp == nil {
		return Result{}, fmt.Errorf("elevenlabs transcribe: empty response")
	}
	return Result{
		Text:         resp.Text,
		LanguageCode: resp.LanguageCode,
	}, nil
}
