package bootstrap

import (
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestNewSandboxProviderEmptyProviderDisablesSandbox(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{})
	if !errors.Is(err, errSandboxProviderNotConfigured) {
		t.Fatalf("newSandboxProvider error = %v, want errSandboxProviderNotConfigured", err)
	}
}

func TestNewSandboxProviderCreatesDaytonaWhenConfigured(t *testing.T) {
	provider, err := newSandboxProvider(&config.Config{
		SandboxProviderID: sandbox.ProviderDaytona,
		DaytonaAPIKey:     "test-key",
	})
	if err != nil {
		t.Fatalf("newSandboxProvider: %v", err)
	}
	if provider.ID() != sandbox.ProviderDaytona {
		t.Fatalf("provider ID = %q, want %q", provider.ID(), sandbox.ProviderDaytona)
	}
}

func TestNewSandboxProviderDaytonaWithoutCredentialsDisablesSandbox(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{
		SandboxProviderID: sandbox.ProviderDaytona,
	})
	if !errors.Is(err, errSandboxProviderNotConfigured) {
		t.Fatalf("newSandboxProvider error = %v, want errSandboxProviderNotConfigured", err)
	}
}

func TestNewSandboxProviderCreatesMicrosandboxWhenConfigured(t *testing.T) {
	provider, err := newSandboxProvider(&config.Config{
		SandboxProviderID:               sandbox.ProviderMicrosandbox,
		MicrosandboxControlURL:          "http://127.0.0.1:8080",
		MicrosandboxControlAPIToken:     "test-token",
		MicrosandboxDefaultPreviewPorts: []int{3000, 7080},
		SandboxesRuntimeBaseImage:       "ghcr.io/usehivy/hivy-sandboxes-runtime:latest",
	})
	if err != nil {
		t.Fatalf("newSandboxProvider: %v", err)
	}
	if provider.ID() != sandbox.ProviderMicrosandbox {
		t.Fatalf("provider ID = %q, want %q", provider.ID(), sandbox.ProviderMicrosandbox)
	}
}

func TestNewSandboxProviderMicrosandboxWithoutCredentialsDisablesSandbox(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{
		SandboxProviderID: sandbox.ProviderMicrosandbox,
	})
	if !errors.Is(err, errSandboxProviderNotConfigured) {
		t.Fatalf("newSandboxProvider error = %v, want errSandboxProviderNotConfigured", err)
	}
}

func TestNewSandboxProviderRejectsUnknownProvider(t *testing.T) {
	_, err := newSandboxProvider(&config.Config{
		SandboxProviderID: "unknown",
		DaytonaAPIKey:     "test-key",
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
