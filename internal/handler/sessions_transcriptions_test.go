package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/transcription"
)

type fakeSessionAssetReader struct {
	data []byte
	key  string
}

func (r *fakeSessionAssetReader) Open(_ context.Context, key string) (io.ReadCloser, error) {
	r.key = key
	return io.NopCloser(bytes.NewReader(r.data)), nil
}

type fakeSessionTranscriber struct {
	text string
	req  transcription.Request
}

func (t *fakeSessionTranscriber) Transcribe(_ context.Context, req transcription.Request) (transcription.Result, error) {
	t.req = req
	t.req.APIKey = append([]byte(nil), req.APIKey...)
	t.req.Audio = append([]byte(nil), req.Audio...)
	return transcription.Result{Text: t.text}, nil
}

func TestIntegration_SessionTranscribeAudio_ReturnsTranscript(t *testing.T) {
	kms := newSystemTaskKMS(t)
	reader := &fakeSessionAssetReader{data: []byte("fake webm audio")}
	transcriber := &fakeSessionTranscriber{text: "Please summarize the launch plan."}
	h := newSessionHarnessWith(t, func(sh *handler.SessionHandler) {
		sh.WithTranscription(kms, reader, transcriber, registry.Global())
	})
	fx := h.seed(t)
	cred := seedSystemCredential(t, h.db, kms, "https://api.elevenlabs.test", "elevenlabs")
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	created := h.createSession(t, fx, fx.owner, "Start")
	asset := seedSessionAudioAsset(t, h, fx, "voice.webm", "audio/webm", int64(len(reader.data)))

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/transcriptions", fx, fx.owner, map[string]any{
		"drive_asset_id": asset.ID.String(),
		"language_code":  "en",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("transcribe status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Text != transcriber.text {
		t.Fatalf("text = %q, want %q", resp.Text, transcriber.text)
	}
	if reader.key != asset.Key {
		t.Fatalf("reader key = %q, want %q", reader.key, asset.Key)
	}
	if transcriber.req.ModelID != "scribe_v2" {
		t.Fatalf("model = %q, want scribe_v2", transcriber.req.ModelID)
	}
	if transcriber.req.BaseURL != cred.BaseURL {
		t.Fatalf("base_url = %q, want %q", transcriber.req.BaseURL, cred.BaseURL)
	}
	if string(transcriber.req.APIKey) != "sk-fake" {
		t.Fatalf("api key was not decrypted")
	}
	if string(transcriber.req.Audio) != string(reader.data) {
		t.Fatalf("audio bytes = %q, want %q", string(transcriber.req.Audio), string(reader.data))
	}
	if transcriber.req.LanguageCode != "en" {
		t.Fatalf("language_code = %q, want en", transcriber.req.LanguageCode)
	}
}

func TestIntegration_SessionTranscribeAudio_RejectsNonParticipant(t *testing.T) {
	kms := newSystemTaskKMS(t)
	reader := &fakeSessionAssetReader{data: []byte("fake webm audio")}
	transcriber := &fakeSessionTranscriber{text: "ignored"}
	h := newSessionHarnessWith(t, func(sh *handler.SessionHandler) {
		sh.WithTranscription(kms, reader, transcriber, registry.Global())
	})
	fx := h.seed(t)
	cred := seedSystemCredential(t, h.db, kms, "https://api.elevenlabs.test", "elevenlabs")
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	created := h.createSession(t, fx, fx.owner, "Start")
	asset := seedSessionAudioAsset(t, h, fx, "voice.webm", "audio/webm", int64(len(reader.data)))

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/transcriptions", fx, fx.viewer, map[string]any{
		"drive_asset_id": asset.ID.String(),
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("transcribe status=%d body=%s", rr.Code, rr.Body.String())
	}
	if reader.key != "" {
		t.Fatalf("asset should not be read for unauthorized user")
	}
}

func TestIntegration_SessionTranscribeAudio_RejectsNonAudioAsset(t *testing.T) {
	kms := newSystemTaskKMS(t)
	reader := &fakeSessionAssetReader{data: []byte("not audio")}
	transcriber := &fakeSessionTranscriber{text: "ignored"}
	h := newSessionHarnessWith(t, func(sh *handler.SessionHandler) {
		sh.WithTranscription(kms, reader, transcriber, registry.Global())
	})
	fx := h.seed(t)
	cred := seedSystemCredential(t, h.db, kms, "https://api.elevenlabs.test", "elevenlabs")
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })
	created := h.createSession(t, fx, fx.owner, "Start")
	asset := seedSessionAudioAsset(t, h, fx, "screenshot.png", "image/png", int64(len(reader.data)))

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/transcriptions", fx, fx.owner, map[string]any{
		"drive_asset_id": asset.ID.String(),
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("transcribe status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func seedSessionAudioAsset(t *testing.T, h *sessionHarness, fx sessionFixture, filename, contentType string, size int64) model.AgentAsset {
	t.Helper()
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             "docker",
		ExternalID:             "sandbox-" + uuid.NewString(),
		RuntimeURL:             "http://runtime.test",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "running",
	}
	if err := h.db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	asset := model.AgentAsset{
		ID:          uuid.New(),
		OrgID:       fx.org.ID,
		AgentID:     fx.agent.ID,
		SandboxID:   sandbox.ID,
		Path:        "uploads",
		Filename:    filename,
		Key:         "pub/e/" + fx.agent.ID.String() + "/uploads/" + filename,
		PublicURL:   "https://assets.test/" + filename,
		ContentType: contentType,
		Bytes:       size,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := h.db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	return asset
}
