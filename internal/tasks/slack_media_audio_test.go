package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/transcription"
)

type fakeSlackTranscriber struct {
	duration float64
}

func (f fakeSlackTranscriber) Transcribe(_ context.Context, _ transcription.Request) (transcription.Result, error) {
	return transcription.Result{Text: "hello from slack", LanguageCode: "en", DurationSeconds: f.duration}, nil
}

func slackAudioTestKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	kms, err := crypto.NewAEADWrapper(context.Background(), base64.StdEncoding.EncodeToString(key), "slack-audio-test")
	if err != nil {
		t.Fatalf("kms: %v", err)
	}
	return kms
}

func TestEnrichSlackAudioEmitsBillableGeneration(t *testing.T) {
	db := connectTestDB(t)
	kms := slackAudioTestKMS(t)

	dek, err := crypto.GenerateDEK()
	if err != nil {
		t.Fatalf("dek: %v", err)
	}
	encKey, err := crypto.EncryptCredential([]byte("sk-fake"), dek)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	wrapped, err := kms.Wrap(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	cred := &model.Credential{
		Label:        "elevenlabs-system-" + uuid.NewString()[:8],
		BaseURL:      "https://api.elevenlabs.test",
		AuthScheme:   "bearer",
		ProviderID:   "elevenlabs",
		EncryptedKey: encKey,
		WrappedDEK:   wrapped,
	}
	if err := db.Create(cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/webm")
		_, _ = w.Write([]byte("fake audio bytes"))
	}))
	defer media.Close()

	enq := &enqueue.MockClient{}
	e := &slackMediaEnricher{
		db:          db,
		kms:         kms,
		registry:    registry.Global(),
		transcriber: fakeSlackTranscriber{duration: 3600},
		enqueuer:    enq,
		httpClient:  media.Client(),
	}

	orgID := uuid.New()
	out := e.enrichSlackAudio(context.Background(), "slack-token", orgID, slackapp.SlackMediaItem{
		Kind:     "audio",
		URL:      media.URL,
		Name:     "voice.webm",
		MimeType: "audio/webm",
	})
	if out == "" {
		t.Fatalf("expected enrichment output")
	}

	var payload ModelUsageWritePayload
	found := false
	for _, task := range enq.Tasks() {
		if task.TypeName != TypeModelUsageWrite {
			continue
		}
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected a model usage write task for the slack audio transcription")
	}
	if payload.Generation.OrgID != orgID {
		t.Fatalf("generation org = %s, want %s", payload.Generation.OrgID, orgID)
	}
	if payload.Generation.ProviderID != "elevenlabs" || payload.Generation.Model != "scribe-v2" {
		t.Fatalf("unexpected provider/model: %+v", payload.Generation)
	}
	if payload.Generation.CredentialID != cred.ID {
		t.Fatalf("credential id = %s, want %s", payload.Generation.CredentialID, cred.ID)
	}
	if payload.Generation.Cost != 0.22 {
		t.Fatalf("cost = %v, want 0.22 for one hour", payload.Generation.Cost)
	}
	if !payload.Generation.IsSystem {
		t.Fatalf("expected system generation for a system credential")
	}
}
