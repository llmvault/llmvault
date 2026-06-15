package sandbox

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
)

func TestSnapshotAliasForImageDerivesFromImageTagAndSize(t *testing.T) {
	got := SnapshotAliasForImage("ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64", "small")
	want := "hivy-sandboxes-runtime-v3-1-18-amd64-small"
	if got != want {
		t.Fatalf("alias = %q, want %q", got, want)
	}
}

func TestAgentRuntimeTemplateRefUsesMicrosandboxSmallSnapshotAlias(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:         ProviderMicrosandbox,
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64",
	}
	got := AgentRuntimeTemplateRef(cfg)
	want := "hivy-sandboxes-runtime-v3-1-18-amd64-small"
	if got != want {
		t.Fatalf("runtime template ref = %q, want %q", got, want)
	}
}

func TestAgentRuntimeTemplateRefLeavesNonMicrosandboxImageRef(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:         ProviderDocker,
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64",
	}
	got := AgentRuntimeTemplateRef(cfg)
	want := cfg.SandboxesRuntimeBaseImage
	if got != want {
		t.Fatalf("runtime template ref = %q, want %q", got, want)
	}
}
