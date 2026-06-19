package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestElevenLabsTranscriber_Transcribe(t *testing.T) {
	var gotPath string
	var gotAPIKey string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("xi-api-key")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
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
	if !bytes.Contains(gotBody, []byte("scribe_v2")) {
		t.Fatalf("body did not include model id: %s", string(gotBody))
	}
	if !strings.Contains(string(gotBody), "ZmFrZSBhdWRpbw==") {
		t.Fatalf("body did not include base64 audio: %s", string(gotBody))
	}
}
