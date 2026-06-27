package sandbox

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
)

func TestRuntimeControlURLRewritesDockerRuntimeOrigin(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{SandboxDockerControlOrigin: "http://host.docker.internal"}
	got := runtimeControlURL(cfg, ProviderDocker, "http://127.0.0.1:33212/healthz?probe=1")
	want := "http://host.docker.internal:33212/healthz?probe=1"
	if got != want {
		t.Fatalf("runtimeControlURL = %q, want %q", got, want)
	}
}

func TestRuntimeControlURLLeavesNonDockerRuntimeURL(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{SandboxDockerControlOrigin: "http://host.docker.internal"}
	raw := "http://127.0.0.1:33212"
	if got := runtimeControlURL(cfg, ProviderMicrosandbox, raw); got != raw {
		t.Fatalf("runtimeControlURL = %q, want %q", got, raw)
	}
}

func TestRuntimeControlURLLeavesRuntimeURLWithoutControlOrigin(t *testing.T) {
	t.Parallel()

	raw := "http://127.0.0.1:33212"
	if got := runtimeControlURL(&config.Config{}, ProviderDocker, raw); got != raw {
		t.Fatalf("runtimeControlURL = %q, want %q", got, raw)
	}
}

func TestRuntimeControlURLLeavesRuntimeURLForInvalidControlOrigin(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{SandboxDockerControlOrigin: "http://host.docker.internal:7080"}
	raw := "http://127.0.0.1:33212"
	if got := runtimeControlURL(cfg, ProviderDocker, raw); got != raw {
		t.Fatalf("runtimeControlURL = %q, want %q", got, raw)
	}
}
