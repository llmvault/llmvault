package control

import (
	"testing"

	"github.com/usehivy/hivy/internal/microsandbox/api"
)

func TestResolveSizeSupportsNano(t *testing.T) {
	got := resolveSize(createSandboxRequest{Size: "nano"})
	want := api.Size{CPU: 1, MemoryMB: 1024, DiskGB: 5}
	if got != want {
		t.Fatalf("resolveSize(nano) = %+v, want %+v", got, want)
	}
}

func TestResolveSizeUsesNanoFloorForExplicitResources(t *testing.T) {
	got := resolveSize(createSandboxRequest{CPU: 1})
	want := api.Size{CPU: 1, MemoryMB: 1024, DiskGB: 5}
	if got != want {
		t.Fatalf("resolveSize(explicit CPU) = %+v, want %+v", got, want)
	}
}
