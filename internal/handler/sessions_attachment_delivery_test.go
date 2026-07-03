package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsSend_AttachmentIDsHydrateAndSendTextContextOnly(t *testing.T) {
	runtime := newSessionRuntimeStub(t, http.StatusOK)
	h, _ := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "First direct turn")
	if created.Session.SandboxID == nil {
		t.Fatalf("created session sandbox_id missing: %+v", created.Session)
	}
	asset := seedSessionImageAsset(t, h, fx, uuid.MustParse(*created.Session.SandboxID), true)
	releaseSessionForNextUserTurn(t, h, created.Session.ID)

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text":           "can you see this image?",
		"attachment_ids": []string{asset.ID.String()},
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	attachments, ok := out.Event.Payload["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("event attachments=%#v, want hydrated attachment", out.Event.Payload["attachments"])
	}
	attachment := attachments[0].(map[string]any)
	if attachment["drive_asset_id"] != asset.ID.String() || attachment["filename"] != asset.Filename {
		t.Fatalf("hydrated attachment=%#v", attachment)
	}
	if runtime.messageCalls != 2 {
		t.Fatalf("runtime message calls=%d, want 2", runtime.messageCalls)
	}
	for _, want := range []string{
		"<attachment name=\"screen.png\"",
		"<important_details>",
		"Shows an orange Hivy logo",
		"<full_description>",
		"Primary category: Product UI",
		"<short_description>",
		"An orange Hivy logo.",
	} {
		if !strings.Contains(runtime.lastMessageText, want) {
			t.Fatalf("runtime text missing %q:\n%s", want, runtime.lastMessageText)
		}
	}
	if len(runtime.lastAttachments) != 0 {
		t.Fatalf("runtime attachments=%#v, want none for described image", runtime.lastAttachments)
	}
}

func TestIntegration_SessionsSend_AttachmentIDWithoutDescriptionFailsClearly(t *testing.T) {
	runtime := newSessionRuntimeStub(t, http.StatusOK)
	h, _ := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "First direct turn")
	if created.Session.SandboxID == nil {
		t.Fatalf("created session sandbox_id missing: %+v", created.Session)
	}
	asset := seedSessionImageAsset(t, h, fx, uuid.MustParse(*created.Session.SandboxID), false)
	releaseSessionForNextUserTurn(t, h, created.Session.ID)

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text":           "can you see this image?",
		"attachment_ids": []string{asset.ID.String()},
	})
	if msg.Code != http.StatusUnprocessableEntity {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	if !strings.Contains(msg.Body.String(), "attachment image description is required") {
		t.Fatalf("message body=%s", msg.Body.String())
	}
	if runtime.messageCalls != 1 {
		t.Fatalf("runtime message calls=%d, want only initial send", runtime.messageCalls)
	}
}

func seedSessionImageAsset(t *testing.T, h *sessionHarness, fx sessionFixture, sandboxID uuid.UUID, described bool) model.AgentAsset {
	t.Helper()
	asset := model.AgentAsset{
		ID:          uuid.New(),
		OrgID:       fx.org.ID,
		AgentID:     fx.agent.ID,
		SandboxID:   &sandboxID,
		Path:        "uploads",
		Filename:    "screen.png",
		Key:         "pub/e/" + fx.agent.ID.String() + "/uploads/screen.png",
		PublicURL:   "https://api.example.test/v1/assets/preview?path=screen.png",
		ContentType: "image/png",
		Bytes:       893227,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if described {
		desc := model.RawJSON(`{"category":"product_ui","confidence":0.98,"rendered_description":"Primary category: Product UI\nSummary:\nAn orange Hivy logo.","analysis":{"summary":"An orange Hivy logo.","important_details":["Shows an orange Hivy logo","Transparent background"]}}`)
		asset.Description = &desc
	}
	if err := h.db.Create(&asset).Error; err != nil {
		t.Fatalf("create session image asset: %v", err)
	}
	return asset
}
