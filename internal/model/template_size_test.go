package model

import "testing"

func TestDefaultAgentSandboxSizeIsNano(t *testing.T) {
	if DefaultAgentSandboxSize != "nano" {
		t.Fatalf("DefaultAgentSandboxSize = %q, want nano", DefaultAgentSandboxSize)
	}
	if got := NormalizeTemplateSize(""); got != "nano" {
		t.Fatalf("NormalizeTemplateSize(empty) = %q, want nano", got)
	}
}

func TestNanoTemplateSize(t *testing.T) {
	if !ValidTemplateSize("nano") {
		t.Fatal("nano should be a valid template size")
	}
	spec, ok := TemplateSizeSpec("nano")
	if !ok {
		t.Fatal("TemplateSizeSpec(nano) missing")
	}
	if spec.CPU != 1 || spec.Memory != 1 || spec.Disk != 5 {
		t.Fatalf("nano spec = %+v, want 1 CPU / 1 GB / 5 GB", spec)
	}
	if got, ok := TemplateSizeForResources(1, 1, 5); !ok || got != "nano" {
		t.Fatalf("TemplateSizeForResources(1,1,5) = %q, %v; want nano, true", got, ok)
	}
}

func TestMicroTemplateSize(t *testing.T) {
	if !ValidTemplateSize("micro") {
		t.Fatal("micro should be a valid template size")
	}
	if got := NormalizeTemplateSize("  MICRO "); got != "micro" {
		t.Fatalf("NormalizeTemplateSize(MICRO) = %q, want micro", got)
	}
	spec, ok := TemplateSizeSpec("micro")
	if !ok {
		t.Fatal("TemplateSizeSpec(micro) missing")
	}
	// micro is the app-tier minimum: 1 CPU, sub-GB memory (recorded as 0 in the
	// GB-granular table), same 5 GB disk floor as nano.
	if spec.CPU != 1 || spec.Memory != 0 || spec.Disk != 5 {
		t.Fatalf("micro spec = %+v, want 1 CPU / 0 GB / 5 GB", spec)
	}
	// micro must stay distinct from nano so the {1,1,5} reverse lookup is stable.
	if got, ok := TemplateSizeForResources(1, 1, 5); !ok || got != "nano" {
		t.Fatalf("TemplateSizeForResources(1,1,5) = %q, %v; want nano (micro must not shadow it)", got, ok)
	}
}
