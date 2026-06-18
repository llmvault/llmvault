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

func TestAgentRuntimeTemplateRefForSizeUsesMicrosandboxSizeSnapshotAlias(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:         ProviderMicrosandbox,
		SandboxesRuntimeBaseImage: "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64",
	}
	got := AgentRuntimeTemplateRefForSize(cfg, "xlarge")
	want := "hivy-sandboxes-runtime-v3-1-18-amd64-xlarge"
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

func TestAgentRuntimeImageRefDerivesProfilesFromImageTag(t *testing.T) {
	cfg := &config.Config{SandboxesRuntimeImageTag: "v3.4.0-amd64"}

	if got, want := AgentRuntimeImageRef(cfg, "default"), "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.4.0-amd64"; got != want {
		t.Fatalf("default runtime image = %q, want %q", got, want)
	}
	if got, want := AgentRuntimeImageRef(cfg, "developer"), "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.4.0-amd64"; got != want {
		t.Fatalf("developer runtime image = %q, want %q", got, want)
	}
}

func TestAgentRuntimeTemplateRefUsesDeveloperMicrosandboxSnapshotAlias(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:        ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "v3.4.0-amd64",
	}

	got := AgentRuntimeTemplateRefForSandboxImageAndSize(cfg, "developer", "large")
	want := "hivy-sandboxes-runtime-developers-v3-4-0-amd64-large"
	if got != want {
		t.Fatalf("developer runtime template ref = %q, want %q", got, want)
	}
}
