package model

import "testing"

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
