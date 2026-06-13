package runner

import "testing"

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
	state, ok := recoverSandboxState("sbx_from_name", "stopped", `{"ports": {"49158": 3001}}`)
	if !ok {
		t.Fatal("expected sandbox state to recover from Hivy sandbox name")
	}
	if state.ID != "sbx_from_name" {
		t.Fatalf("ID = %q, want sbx_from_name", state.ID)
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
	labels := hivyLabels("sbx_label", map[string]string{"org_id": "org_1"})
	if labels[hivyManagedLabel] != "true" || labels[sandboxIDLabel] != "sbx_label" || labels["org_id"] != "org_1" {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestHivyLabelsDoesNotMarkNonSandboxBuildsAsSandboxes(t *testing.T) {
	labels := hivyLabels("snp_build", nil)
	if labels[hivyManagedLabel] != "true" {
		t.Fatalf("labels = %#v", labels)
	}
	if labels[sandboxIDLabel] != "" {
		t.Fatalf("non-sandbox build should not get sandbox_id label: %#v", labels)
	}
}
