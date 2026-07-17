package main

import "testing"

func TestSandboxParamsBootstrapsRuntimeWithoutConfigPush(t *testing.T) {
	params := sandboxParams("runtime-snapshot", "runtime-secret")

	if params.Snapshot != "runtime-snapshot" {
		t.Fatalf("snapshot = %q, want runtime-snapshot", params.Snapshot)
	}
	if got := params.EnvVars[runtimeSecretEnv]; got != "runtime-secret" {
		t.Fatalf("%s = %q, want runtime-secret", runtimeSecretEnv, got)
	}
	if params.User != daytonaUser {
		t.Fatalf("user = %q, want %q", params.User, daytonaUser)
	}
	if params.Public {
		t.Fatal("sandbox preview must remain private")
	}
	if params.NetworkBlockAll {
		t.Fatal("sandbox outbound network must be enabled")
	}
	if got := params.EnvVars["HOME"]; got != daytonaHome {
		t.Fatalf("HOME = %q, want %q", got, daytonaHome)
	}
	if got := params.EnvVars["HIVY_WORKSPACE_ROOT"]; got != daytonaWorkspaceRoot {
		t.Fatalf("HIVY_WORKSPACE_ROOT = %q, want %q", got, daytonaWorkspaceRoot)
	}
	if got := params.EnvVars["HIVY_DB_PATH"]; got != daytonaDBPath {
		t.Fatalf("HIVY_DB_PATH = %q, want %q", got, daytonaDBPath)
	}
	if runtimePort != 7080 {
		t.Fatalf("runtime port = %d, want 7080", runtimePort)
	}
	if runtimeHealthPath != "/healthz" {
		t.Fatalf("runtime health path = %q, want /healthz", runtimeHealthPath)
	}
}

func TestGenerateRuntimeSecret(t *testing.T) {
	first, err := generateRuntimeSecret()
	if err != nil {
		t.Fatalf("generate first runtime secret: %v", err)
	}
	second, err := generateRuntimeSecret()
	if err != nil {
		t.Fatalf("generate second runtime secret: %v", err)
	}

	if len(first) != 64 {
		t.Fatalf("runtime secret length = %d, want 64", len(first))
	}
	if first == second {
		t.Fatal("generated runtime secrets must differ")
	}
}
