package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
)

func TestAgentSessionsAudioTranscriptionE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	agentSessionsEnsureSystemOpenRouterCredential(t)
	agentSessionsEnsureSystemElevenLabsCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-audio-transcription-e2e-password"
	ownerEmail := "agent-audio-transcription-" + runID + "@example.com"

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Audio Transcription "+runID)
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	agent := agentSessionsCreateAudioTranscriptionAgent(t, ctx, apiBase, token, orgID, runID)
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "audio-transcription-"+runID, agent.ID)
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channel.ID, "")
	if session.Session.ID == "" || session.Session.SandboxID == nil {
		t.Fatalf("session did not create a per-session sandbox: %+v", session)
	}

	audio := agentSessionsAudioFixture(t)
	asset := agentSessionsUploadAudioAsset(t, ctx, apiBase, token, orgID, agent.ID, audio)
	transcript := agentSessionsTranscribeAudio(t, ctx, apiBase, token, orgID, session.Session.ID, asset.ID)
	t.Logf("audio transcription transcript=%q asset_id=%s", transcript.Text, asset.ID)
	assertAgentSessionsTranscriptContains(t, transcript.Text, "voice recording test alpha banana")
}

type agentSessionsUploadedAsset struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
}

type agentSessionsTranscriptionResponse struct {
	Text string `json:"text"`
}

func agentSessionsCreateAudioTranscriptionAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, runID string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	payload := map[string]any{
		"name":             "Audio transcription E2E " + runID,
		"instructions":     "This agent exists for the audio transcription E2E.",
		"model":            agentruntime.DefaultAgentModel,
		"available_models": []string{agentruntime.DefaultAgentModel},
		"sandbox_strategy": "per_session",
	}
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents", token, orgID, payload, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("audio transcription agent create returned empty agent: %+v", out)
	}
	return out.Agent
}

func agentSessionsAudioFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("testdata", "audio_transcription_flagship.wav")
	audio, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audio fixture %s: %v", path, err)
	}
	return audio
}

func agentSessionsUploadAudioAsset(t *testing.T, ctx context.Context, baseURL, token, orgID, agentID string, audio []byte) agentSessionsUploadedAsset {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("agent_id", agentID); err != nil {
		t.Fatalf("write agent_id: %v", err)
	}
	if err := writer.WriteField("path", "uploads"); err != nil {
		t.Fatalf("write path: %v", err)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", multipart.FileContentDisposition("file", "audio_transcription_flagship.wav"))
	header.Set("Content-Type", "audio/wav")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create audio file part: %v", err)
	}
	if _, err := part.Write(audio); err != nil {
		t.Fatalf("write audio file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close audio multipart body: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/assets/upload", &body)
	if err != nil {
		t.Fatalf("build audio upload request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload audio asset failed: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload audio asset status=%d want=%d body=%s", resp.StatusCode, http.StatusCreated, raw)
	}
	var out agentSessionsUploadedAsset
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode audio upload response: %v\n%s", err, raw)
	}
	if out.ID == "" || out.ContentType != "audio/wav" || out.Bytes <= 0 {
		t.Fatalf("unexpected audio upload response: %+v", out)
	}
	return out
}

func agentSessionsTranscribeAudio(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, assetID string) agentSessionsTranscriptionResponse {
	t.Helper()
	var out agentSessionsTranscriptionResponse
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/sessions/"+sessionID+"/transcriptions", token, orgID, map[string]any{
		"drive_asset_id": assetID,
		"language_code":  "en",
	}, http.StatusOK, &out)
	if strings.TrimSpace(out.Text) == "" {
		t.Fatalf("transcription returned empty text: %+v", out)
	}
	return out
}

var agentSessionsTranscriptTokenPattern = regexp.MustCompile(`[a-z0-9]+`)

func assertAgentSessionsTranscriptContains(t *testing.T, transcript, phrase string) {
	t.Helper()
	normalized := strings.Join(agentSessionsTranscriptTokenPattern.FindAllString(strings.ToLower(transcript), -1), " ")
	want := strings.Join(agentSessionsTranscriptTokenPattern.FindAllString(strings.ToLower(phrase), -1), " ")
	if !strings.Contains(normalized, want) {
		t.Fatalf("transcript %q normalized to %q, want phrase %q", transcript, normalized, want)
	}
}
