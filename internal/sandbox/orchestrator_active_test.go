package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestGetRuntimeClientStartsDockerSandbox(t *testing.T) {
	db := connectSandboxTestDB(t)
	encKey := sandboxTestSymmetricKey(t)
	encryptedSecret, err := encKey.EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}

	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(runtime.Close)

	for _, initialStatus := range []SandboxStatus{StatusStopped, StatusStarting} {
		t.Run(string(initialStatus), func(t *testing.T) {
			provider := &agentCreateProvider{
				endpoint:   runtime.URL,
				providerID: ProviderDocker,
			}
			orchestrator := NewOrchestrator(db, provider, encKey, &config.Config{
				SandboxProviderID: ProviderDocker,
			})
			sb := model.Sandbox{
				ID:                     uuid.New(),
				ProviderID:             ProviderDocker,
				ExternalID:             "docker-" + string(initialStatus) + "-sandbox",
				RuntimeURL:             runtime.URL,
				EncryptedRuntimeSecret: encryptedSecret,
				Status:                 string(initialStatus),
			}
			if err := db.WithContext(t.Context()).Create(&sb).Error; err != nil {
				t.Fatalf("create %s sandbox: %v", initialStatus, err)
			}
			t.Cleanup(func() {
				db.WithContext(context.Background()).Delete(&model.Sandbox{}, "id = ?", sb.ID)
			})

			if _, err := orchestrator.GetRuntimeClient(t.Context(), &sb); err != nil {
				t.Fatalf("GetRuntimeClient: %v", err)
			}

			if len(provider.started) != 1 || provider.started[0] != sb.ExternalID {
				t.Fatalf("provider starts = %v, want [%s]", provider.started, sb.ExternalID)
			}
			if sb.Status != string(StatusRunning) {
				t.Fatalf("sandbox status = %q, want %q", sb.Status, StatusRunning)
			}
			if sb.RuntimeURL != runtime.URL {
				t.Fatalf("sandbox runtime URL = %q, want %q", sb.RuntimeURL, runtime.URL)
			}

			var persisted model.Sandbox
			if err := db.WithContext(t.Context()).First(&persisted, "id = ?", sb.ID).Error; err != nil {
				t.Fatalf("reload sandbox: %v", err)
			}
			if persisted.Status != string(StatusRunning) {
				t.Fatalf("persisted status = %q, want %q", persisted.Status, StatusRunning)
			}
		})
	}
}
