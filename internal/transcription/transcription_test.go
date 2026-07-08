package transcription

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/billing"
)

func TestElevenLabsTranscriber_Transcribe(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotContentType string
	var gotModelID string
	var gotLanguageCode string
	var gotFilename string
	var gotFileContentType string
	var gotFileBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("xi-api-key")
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart body: %v", err)
		}
		gotModelID = r.FormValue("model_id")
		gotLanguageCode = r.FormValue("language_code")
		files := r.MultipartForm.File["file"]
		if len(files) != 1 {
			t.Fatalf("file parts = %d, want 1", len(files))
		}
		gotFilename = files[0].Filename
		gotFileContentType = files[0].Header.Get("Content-Type")
		file, err := files[0].Open()
		if err != nil {
			t.Fatalf("open file part: %v", err)
		}
		defer file.Close()
		gotFileBytes, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file part: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"text":                 "Launch notes transcribed",
			"language_code":        "en",
			"language_probability": 0.99,
			"words":                []any{},
		}); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	result, err := NewElevenLabsTranscriber(srv.Client(), time.Second).Transcribe(context.Background(), Request{
		APIKey:       []byte("sk-test"),
		BaseURL:      srv.URL,
		ModelID:      "scribe_v2",
		Audio:        []byte("fake audio"),
		Filename:     "voice.webm",
		ContentType:  "audio/webm",
		LanguageCode: "en",
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if result.Text != "Launch notes transcribed" || result.LanguageCode != "en" {
		t.Fatalf("result = %+v", result)
	}
	if gotPath != "/v1/speech-to-text" {
		t.Fatalf("path = %q, want /v1/speech-to-text", gotPath)
	}
	if gotAPIKey != "sk-test" {
		t.Fatalf("xi-api-key = %q, want sk-test", gotAPIKey)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Fatalf("content-type = %q, want multipart form", gotContentType)
	}
	if gotModelID != "scribe_v2" {
		t.Fatalf("model_id = %q, want scribe_v2", gotModelID)
	}
	if gotLanguageCode != "en" {
		t.Fatalf("language_code = %q, want en", gotLanguageCode)
	}
	if gotFilename != "voice.webm" {
		t.Fatalf("filename = %q, want voice.webm", gotFilename)
	}
	if gotFileContentType != "audio/webm" {
		t.Fatalf("file content-type = %q, want audio/webm", gotFileContentType)
	}
	if string(gotFileBytes) != "fake audio" {
		t.Fatalf("file bytes = %q, want raw audio bytes", string(gotFileBytes))
	}
}

func TestElevenLabsTranscriber_UsesAudioDurationSecsOverWordTimestamps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"text":                "hello",
			"language_code":       "en",
			"audio_duration_secs": 42.5,
			"words": []map[string]any{
				{"start": 0.0, "end": 3.0},
				{"start": 3.0, "end": 7.0},
			},
		}); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	result, err := NewElevenLabsTranscriber(srv.Client(), time.Second).Transcribe(context.Background(), Request{
		APIKey:  []byte("sk-test"),
		BaseURL: srv.URL,
		Audio:   []byte("fake audio"),
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if result.DurationSeconds != 42.5 {
		t.Fatalf("DurationSeconds = %v, want 42.5 (audio_duration_secs, not max word end)", result.DurationSeconds)
	}
}

func TestElevenLabsTranscriber_FallsBackToWordTimestamps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"text":          "hello",
			"language_code": "en",
			"words": []map[string]any{
				{"start": 0.0, "end": 3.0},
				{"start": 3.0, "end": 7.0},
			},
		}); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	result, err := NewElevenLabsTranscriber(srv.Client(), time.Second).Transcribe(context.Background(), Request{
		APIKey:  []byte("sk-test"),
		BaseURL: srv.URL,
		Audio:   []byte("fake audio"),
	})
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if result.DurationSeconds != 7.0 {
		t.Fatalf("DurationSeconds = %v, want 7.0 (max word end fallback)", result.DurationSeconds)
	}
}

func TestCostUSD(t *testing.T) {
	if got := CostUSD(3600); got != 0.22 {
		t.Fatalf("CostUSD(3600) = %v, want 0.22", got)
	}
	if got := billing.CostUSDToCredits(CostUSD(3600)); got != 220 {
		t.Fatalf("credits for one hour = %d, want 220", got)
	}
	if got := CostUSD(0); got != billing.CreditUSDValue {
		t.Fatalf("CostUSD(0) = %v, want floor %v", got, billing.CreditUSDValue)
	}
	if got := billing.CostUSDToCredits(CostUSD(0)); got != 1 {
		t.Fatalf("credits for zero-duration success = %d, want 1", got)
	}
}

func TestElevenLabsTranscriber_TranscribeIncludesUpstreamErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"detail":"invalid file"}`)
	}))
	defer srv.Close()

	_, err := NewElevenLabsTranscriber(srv.Client(), time.Second).Transcribe(context.Background(), Request{
		APIKey:  []byte("sk-test"),
		BaseURL: srv.URL,
		Audio:   []byte("fake audio"),
	})
	if err == nil {
		t.Fatal("Transcribe succeeded, want error")
	}
	if !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "invalid file") {
		t.Fatalf("error = %q, want status and response body", err.Error())
	}
}
