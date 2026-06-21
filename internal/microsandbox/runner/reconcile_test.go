package runner

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/microsandbox/config"
)

func TestRecoverSandboxStateFromLabelsAndPorts(t *testing.T) {
	state, ok := recoverSandboxState("anything", "running", `{
		"labels": {"org_id": "org_1", "sandbox_id": "sbx_test"},
		"ports": {"49152": 3000, "49153": 5173}
	}`)
	if !ok {
		t.Fatal("expected sandbox state to recover")
	}
	if state.ID != "sbx_test" {
		t.Fatalf("ID = %q, want sbx_test", state.ID)
	}
	if state.Ports[3000] != 49152 || state.Ports[5173] != 49153 {
		t.Fatalf("ports = %#v", state.Ports)
	}
	if state.Labels["org_id"] != "org_1" {
		t.Fatalf("labels = %#v", state.Labels)
	}
}

func TestRecoverSandboxStateFromPortBindings(t *testing.T) {
	state, ok := recoverSandboxState("anything", "running", `{
		"labels": {"sandbox_id": "sbx_bindings"},
		"port_bindings": [
			{"bind": "127.0.0.1", "host_port": 49154, "guest_port": 8080},
			{"bind": "127.0.0.1", "host_port": 49155, "guest_port": 5353, "protocol": "udp"}
		],
		"network": {
			"ports": [{"host_port": 49156, "guest_port": 8000, "protocol": "tcp"}],
			"port_bindings": [{"host_port": 49157, "guest_port": 9000, "protocol": "tcp"}]
		}
	}`)
	if !ok {
		t.Fatal("expected sandbox state to recover")
	}
	if state.Ports[8080] != 49154 || state.Ports[8000] != 49156 || state.Ports[9000] != 49157 {
		t.Fatalf("ports = %#v", state.Ports)
	}
	if _, ok := state.Ports[5353]; ok {
		t.Fatalf("udp binding should not be recovered as an HTTP preview port: %#v", state.Ports)
	}
}

func TestRecoverSandboxStateFallsBackToHivyName(t *testing.T) {
	state, ok := recoverSandboxState("abc123xy", "stopped", `{"ports": {"49158": 3001}}`)
	if !ok {
		t.Fatal("expected sandbox state to recover from Hivy sandbox name")
	}
	if state.ID != "abc123xy" {
		t.Fatalf("ID = %q, want abc123xy", state.ID)
	}
	if state.Status != "stopped" {
		t.Fatalf("Status = %q, want stopped", state.Status)
	}
	if state.Ports[3001] != 49158 {
		t.Fatalf("ports = %#v", state.Ports)
	}
}

func TestRecoverSandboxStateSkipsUnmanagedSandbox(t *testing.T) {
	_, ok := recoverSandboxState("personal-dev", "running", `{"ports": {"49159": 3000}}`)
	if ok {
		t.Fatal("expected unmanaged sandbox to be skipped")
	}
}

func TestHivyLabelsAddsRecoveryLabels(t *testing.T) {
	labels := hivyLabels("abc123xy", map[string]string{"org_id": "org_1"})
	if labels[hivyManagedLabel] != "true" || labels[sandboxIDLabel] != "abc123xy" || labels["org_id"] != "org_1" {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestHivyLabelsDoesNotMarkNonSandboxBuildsAsSandboxes(t *testing.T) {
	labels := hivyLabels("bld_build", nil)
	if labels[hivyManagedLabel] != "true" {
		t.Fatalf("labels = %#v", labels)
	}
	if labels[sandboxIDLabel] != "" {
		t.Fatalf("non-sandbox build should not get sandbox_id label: %#v", labels)
	}
}

func TestVolumeNamesIncludeLegacyDockerDataVolume(t *testing.T) {
	if got := workspaceVolumeName("abc123xy"); got != "hivy-abc123xy" {
		t.Fatalf("workspace volume = %q", got)
	}
	if got := dockerDataVolumeName("abc123xy"); got != "hivy-docker-abc123xy" {
		t.Fatalf("legacy docker data volume = %q", got)
	}
}

func TestSandboxVolumeSizesReserveRootOverlayInsideDiskBudget(t *testing.T) {
	rootOverlay, workspace := sandboxVolumeSizesMiB(40)
	if rootOverlay != 8*1024 || workspace != 32*1024 {
		t.Fatalf("40GB split = root %d workspace %d, want 8192/32768", rootOverlay, workspace)
	}

	rootOverlay, workspace = sandboxVolumeSizesMiB(10)
	if rootOverlay != 4*1024 || workspace != 6*1024 {
		t.Fatalf("10GB split = root %d workspace %d, want 4096/6144", rootOverlay, workspace)
	}

	rootOverlay, workspace = sandboxVolumeSizesMiB(160)
	if rootOverlay != maxRootOverlayMiB || workspace != 144*1024 {
		t.Fatalf("160GB split = root %d workspace %d, want %d/147456", rootOverlay, workspace, maxRootOverlayMiB)
	}
}

func TestSandboxEnvWithStorageDefaults(t *testing.T) {
	env := sandboxEnvWithStorageDefaults(map[string]string{"TMPDIR": "/custom/tmp"})
	if env["HIVY_SANDBOX_DATA_ROOT"] != sandboxDataRootPath {
		t.Fatalf("HIVY_SANDBOX_DATA_ROOT = %q", env["HIVY_SANDBOX_DATA_ROOT"])
	}
	if env["HIVY_DOCKER_DATA_ROOT"] != dockerDataRootPath {
		t.Fatalf("HIVY_DOCKER_DATA_ROOT = %q", env["HIVY_DOCKER_DATA_ROOT"])
	}
	if env["TMPDIR"] != "/custom/tmp" {
		t.Fatalf("TMPDIR override = %q", env["TMPDIR"])
	}
	if env["TEMP"] != sandboxTmpPath || env["TMP"] != sandboxTmpPath || env["DOCKER_TMPDIR"] != sandboxTmpPath+"/docker" {
		t.Fatalf("temp defaults = %#v", env)
	}
	if _, ok := env["HIVY_RUNTIME_START_DOCKERD"]; ok {
		t.Fatalf("HIVY_RUNTIME_START_DOCKERD should never be set by runner defaults: %#v", env)
	}
}

func TestSnapshotRoutesAreNotRegistered(t *testing.T) {
	s := &Server{cfg: config.Config{RunnerAPIToken: "runner-token"}, backend: NewMockBackend()}
	req := httptest.NewRequest(http.MethodPost, "/v1/snapshots", nil)
	req.Header.Set("Authorization", "Bearer runner-token")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("snapshot route status = %d, want 404", rec.Code)
	}
}
