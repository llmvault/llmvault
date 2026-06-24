package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsCreate_UsesModelDefinition(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions", fx, fx.owner, map[string]any{
		"channel_id": fx.channel.ID.String(),
		"text":       "Use the selected model",
		"model_definition": map[string]any{
			"model_id":         "deepseek-v4-flash",
			"reasoning_effort": "medium",
		},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rr.Code, rr.Body.String())
	}
	out := decodeSessionMutation(t, rr)
	var stored model.Session
	if err := h.db.First(&stored, "id = ?", out.Session.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if stored.Model != "deepseek-v4-flash" {
		t.Fatalf("session model = %q, want deepseek-v4-flash", stored.Model)
	}
	if stored.ReasoningEffort != "medium" {
		t.Fatalf("session reasoning_effort = %q, want medium", stored.ReasoningEffort)
	}
}
