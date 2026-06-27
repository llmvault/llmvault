package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

func TestIntegration_SessionsRespondToInput_ProxiesStructuredAnswerToRuntime(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Ask me later")
	runtimeSecret := "runtime-question-" + uuid.NewString()
	var gotPath string
	var gotAuthorization string
	var gotBody map[string]any
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode runtime request: %v", err)
		}
		writeJSON(t, w, map[string]any{
			"session_id":          created.Session.ID,
			"question_request_id": "question-request-1",
			"state":               "answered",
		})
	}))
	defer runtime.Close()
	attachSessionSandbox(t, h, fx, created.Session.ID, runtime.URL, runtimeSecret)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/input-responses", fx, fx.owner, map[string]any{
		"question_request_id": "question-request-1",
		"answers": map[string]any{
			"deployment_path": map[string]any{"answers": []string{"Ship It"}},
		},
	})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("input response status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/sessions/"+created.Session.ID+"/questions/question-request-1/answer" {
		t.Fatalf("runtime path=%q", gotPath)
	}
	if gotAuthorization != "Bearer "+runtimeSecret {
		t.Fatalf("runtime authorization=%q", gotAuthorization)
	}
	answers, ok := gotBody["answers"].(map[string]any)
	if !ok {
		t.Fatalf("runtime answers=%#v", gotBody["answers"])
	}
	answer, ok := answers["deployment_path"].(map[string]any)
	if !ok {
		t.Fatalf("deployment_path answer=%#v", answers["deployment_path"])
	}
	selected, ok := answer["answers"].([]any)
	if !ok || len(selected) != 1 || selected[0] != "Ship It" {
		t.Fatalf("selected answer=%#v", answer["answers"])
	}
	if gotBody["user"] != fx.owner.ID.String() || gotBody["user_display_name"] != fx.owner.Name {
		t.Fatalf("runtime user fields=%#v", gotBody)
	}
	var queueCount int64
	if err := h.db.Model(&model.SessionMessageQueue{}).
		Where("session_id = ?", created.Session.ID).
		Count(&queueCount).Error; err != nil {
		t.Fatalf("count queue rows: %v", err)
	}
	if queueCount != 0 {
		t.Fatalf("queue rows=%d, want none", queueCount)
	}
}

func TestIntegration_SessionsRespondToInput_MapsRuntimeQuestionConflict(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Ask me later")
	runtimeSecret := "runtime-question-" + uuid.NewString()
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "question request has already been answered", http.StatusConflict)
	}))
	defer runtime.Close()
	attachSessionSandbox(t, h, fx, created.Session.ID, runtime.URL, runtimeSecret)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/input-responses", fx, fx.owner, map[string]any{
		"question_request_id": "question-request-1",
		"answers": map[string]any{
			"deployment_path": map[string]any{"answers": []string{"Ship It"}},
		},
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("input response status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func attachSessionSandbox(t *testing.T, h *sessionHarness, fx sessionFixture, sessionID, runtimeURL, runtimeSecret string) {
	t.Helper()
	encSecret, err := sessionTestEncKey(t).EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             sandboxpkg.ProviderMicrosandbox,
		ExternalID:             "question-runtime-" + uuid.NewString(),
		RuntimeURL:             runtimeURL,
		EncryptedRuntimeSecret: encSecret,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", sessionID).
		Update("sandbox_id", sb.ID).Error; err != nil {
		t.Fatalf("attach sandbox: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode runtime response: %v", err)
	}
}
