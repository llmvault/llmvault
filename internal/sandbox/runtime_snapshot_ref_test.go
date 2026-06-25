package sandbox

import (
	"testing"

	"github.com/usehivy/hivy/internal/config"
)

func TestAgentRuntimeTemplateRefUsesMicrosandboxRuntimeImage(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:        ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "v3.1.18-amd64",
	}
	got := AgentRuntimeTemplateRef(cfg)
	want := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64"
	if got != want {
		t.Fatalf("runtime template ref = %q, want %q", got, want)
	}
}

func TestAgentRuntimeTemplateRefForSizeStillUsesRuntimeImage(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:        ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "v3.1.18-amd64",
	}
	got := AgentRuntimeTemplateRefForSize(cfg, "xlarge")
	want := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64"
	if got != want {
		t.Fatalf("runtime template ref = %q, want %q", got, want)
	}
}

func TestAgentRuntimeTemplateRefLeavesNonMicrosandboxImageRef(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:        ProviderDocker,
		SandboxesRuntimeImageTag: "v3.1.18-amd64",
	}
	got := AgentRuntimeTemplateRef(cfg)
	want := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.18-amd64"
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

func TestAgentRuntimeTemplateRefUsesDeveloperMicrosandboxRuntimeImage(t *testing.T) {
	cfg := &config.Config{
		SandboxProviderID:        ProviderMicrosandbox,
		SandboxesRuntimeImageTag: "v3.4.0-amd64",
	}

	got := AgentRuntimeTemplateRefForSandboxImageAndSize(cfg, "developer", "large")
	want := "ghcr.io/usehivy/hivy-sandboxes-runtime-developers:v3.4.0-amd64"
	if got != want {
		t.Fatalf("developer runtime template ref = %q, want %q", got, want)
	}
}
