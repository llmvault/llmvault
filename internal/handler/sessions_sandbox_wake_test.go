package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	sandboxpkg "github.com/usehivy/hivy/internal/sandbox"
)

func TestIntegration_SandboxAccessMintsWithoutGoWake(t *testing.T) {
	runtime := newSessionSyncRuntime(t, http.StatusOK)
	h, provider := newSessionRuntimeHarness(t, runtime, nil)
	fx := h.seed(t)
	seedAlwaysOnRuntimeSandbox(t, h, fx, runtime.server.URL, "running")
	created := h.createSession(t, fx, fx.owner, "Access should wake")
	sb := attachStoppedSessionSandbox(t, h, fx, created.Session.ID, runtime.server.URL)

	rr := h.doJSON(t, http.MethodPost, "/v1/sessions/"+created.Session.ID+"/sandbox-access", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("sandbox access status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(provider.started) != 0 {
		t.Fatalf("provider started=%v, want no Go API wake", provider.started)
	}
	var access struct {
		SandboxID      string `json:"sandbox_id"`
		SandboxBaseURL string `json:"sandbox_base_url"`
		Token          string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &access); err != nil {
		t.Fatalf("decode sandbox access: %v\n%s", err, rr.Body.String())
	}
	if access.SandboxID != sb.ID.String() || access.SandboxBaseURL != runtime.server.URL || access.Token == "" {
		t.Fatalf("bad sandbox access: %+v", access)
	}
}

func attachStoppedSessionSandbox(t *testing.T, h *sessionHarness, fx sessionFixture, sessionID, runtimeURL string) model.Sandbox {
	t.Helper()
	encSecret, err := sessionTestEncKey(t).EncryptString("runtime-wake-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             sandboxpkg.ProviderMicrosandbox,
		ExternalID:             "session-wake-" + sessionID[:8],
		RuntimeURL:             runtimeURL,
		EncryptedRuntimeSecret: encSecret,
		Status:                 "stopped",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := h.db.Model(&model.Session{}).Where("id = ?", sessionID).Update("sandbox_id", sb.ID).Error; err != nil {
		t.Fatalf("attach sandbox: %v", err)
	}
	return sb
}
