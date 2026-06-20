package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestIntegration_SessionsSend_AllowsStructuredPayloadOnlyMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Kick off the review")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "",
		"raw": map[string]any{
			"code_line_comments": []any{
				map[string]any{
					"id":           "comment-1",
					"source_kind":  "review",
					"path":         "apps/web/lib/diffs-theme.ts",
					"display_path": "apps/web/lib/diffs-theme.ts",
					"line_number":  148,
					"side":         "additions",
					"body":         "Use the HeroUI token here.",
					"created_at":   float64(1781900000000),
				},
			},
		},
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if out.Event == nil {
		t.Fatalf("missing event: %+v", out)
	}
	if out.Event.Payload["text"] != "" {
		t.Fatalf("stored text=%#v, want empty string", out.Event.Payload["text"])
	}
	if comments, ok := out.Event.Payload["code_line_comments"].([]any); !ok || len(comments) != 1 {
		t.Fatalf("stored code_line_comments=%#v", out.Event.Payload["code_line_comments"])
	}
}

func TestIntegration_SessionsSend_AllowsAttachmentOnlyMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Kick off the review")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "",
		"raw": map[string]any{
			"attachments": []any{
				map[string]any{
					"drive_asset_id":        uuid.NewString(),
					"filename":              "screen.png",
					"asset_url":             "https://api.example.test/assets/screen.png",
					"content_type":          "image/png",
					"rendered_description":  "Primary category: Product UI",
					"analysis_model":        "test-model",
					"analysis_generated_at": "2026-06-20T00:00:00Z",
				},
			},
		},
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if out.Event == nil {
		t.Fatalf("missing event: %+v", out)
	}
	if out.Event.Payload["text"] != "" {
		t.Fatalf("stored text=%#v, want empty string", out.Event.Payload["text"])
	}
	if attachments, ok := out.Event.Payload["attachments"].([]any); !ok || len(attachments) != 1 {
		t.Fatalf("stored attachments=%#v", out.Event.Payload["attachments"])
	}
}
