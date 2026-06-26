package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsSend_AllowsStructuredPayloadOnlyMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Kick off the review")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "",
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
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if out.Event == nil {
		t.Fatalf("event=nil, want backend-owned user event")
	}
	if out.Event.Payload["text"] != "" {
		t.Fatalf("event text=%#v, want empty string", out.Event.Payload["text"])
	}
	if comments, ok := out.Event.Payload["code_line_comments"].([]any); !ok || len(comments) != 1 {
		t.Fatalf("event code_line_comments=%#v", out.Event.Payload["code_line_comments"])
	}
	row := latestSessionMessageQueueRow(t, h, created.Session.ID)
	if row.MessagePayload["text"] != "" {
		t.Fatalf("stored text=%#v, want empty string", row.MessagePayload["text"])
	}
	if comments, ok := row.MessagePayload["code_line_comments"].([]any); !ok || len(comments) != 1 {
		t.Fatalf("stored code_line_comments=%#v", row.MessagePayload["code_line_comments"])
	}
}

func TestIntegration_SessionsSend_AllowsArtifactCommentsOnlyMessage(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Review the canvas")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "",
		"artifact_comments": []any{
			map[string]any{
				"artifact_id":   "artifact-1",
				"artifact_name": "Homepage",
				"selector":      "hero",
				"body":          "Make the headline more direct.",
			},
		},
	})
	if msg.Code != http.StatusAccepted {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
	out := decodeSessionMutation(t, msg)
	if comments, ok := out.Event.Payload["artifact_comments"].([]any); !ok || len(comments) != 1 {
		t.Fatalf("event artifact_comments=%#v", out.Event.Payload["artifact_comments"])
	}
	row := latestSessionMessageQueueRow(t, h, created.Session.ID)
	if comments, ok := row.MessagePayload["artifact_comments"].([]any); !ok || len(comments) != 1 {
		t.Fatalf("stored artifact_comments=%#v", row.MessagePayload["artifact_comments"])
	}
}

func TestIntegration_SessionsSend_RejectsRawEnvelope(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Kick off the review")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/messages", fx, fx.owner, map[string]any{
		"text": "",
		"raw": map[string]any{
			"code_line_comments": []any{
				map[string]any{"body": "old shape"},
			},
		},
	})
	if msg.Code != http.StatusBadRequest {
		t.Fatalf("message status=%d body=%s", msg.Code, msg.Body.String())
	}
}

func TestIntegration_SessionsRespondToInput_RejectsLegacyClientEventID(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Ask me later")

	msg := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/input-responses", fx, fx.owner, map[string]any{
		"request_id":      "question-1",
		"text":            "Use option A",
		"client_event_id": "legacy-input-id",
	})
	if msg.Code != http.StatusBadRequest {
		t.Fatalf("input response status=%d body=%s", msg.Code, msg.Body.String())
	}
}

func latestSessionMessageQueueRow(t *testing.T, h *sessionHarness, sessionID string) model.SessionMessageQueue {
	t.Helper()
	var row model.SessionMessageQueue
	if err := h.db.Where("session_id = ?", sessionID).
		Order("sequence_number DESC").
		First(&row).Error; err != nil {
		t.Fatalf("load latest queue row: %v", err)
	}
	return row
}
