package microsandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/sandbox"
)

func TestDriverUpgradeSandboxPostsUpgradeRequest(t *testing.T) {
	var upgradeReq map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sandboxes/sbx_existing/upgrade" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&upgradeReq); err != nil {
			t.Fatalf("decode upgrade request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ID": "sbx_existing", "Status": "running"})
	}))
	defer server.Close()

	driver, err := NewDriver(Config{
		ControlURL:   server.URL,
		APIToken:     "test-token",
		RuntimeImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:latest",
	})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}
	info, err := driver.UpgradeSandbox(context.Background(), "sbx_existing", sandbox.UpgradeSandboxOpts{
		Name:         "agent-upgrade",
		TemplateRef:  "ghcr.io/usehivy/hivy-sandboxes-runtime:v2",
		Labels:       map[string]string{"org_id": "org_123", "agent_id": "agent_123"},
		EnvVars:      map[string]string{"HIVY_RUNTIME_SECRET": "secret"},
		CPU:          2,
		Memory:       4,
		Disk:         40,
		ExposedPorts: []int{3000},
	})
	if err != nil {
		t.Fatalf("UpgradeSandbox: %v", err)
	}
	if info.ExternalID != "sbx_existing" || info.Status != sandbox.StatusRunning {
		t.Fatalf("UpgradeSandbox returned %+v", info)
	}
	if upgradeReq["image_ref"] != "ghcr.io/usehivy/hivy-sandboxes-runtime:v2" {
		t.Fatalf("image_ref = %v", upgradeReq["image_ref"])
	}
	if upgradeReq["memory_mb"] != float64(4096) {
		t.Fatalf("memory_mb = %v, want 4096", upgradeReq["memory_mb"])
	}
	assertRuntimeHealthCheck(t, upgradeReq, sandbox.AgentSandboxPort)
}
