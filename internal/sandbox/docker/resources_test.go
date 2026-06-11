package docker

import "testing"

// TestResourceLimitsAppliesDefaults verifies that zero-valued CreateSandboxOpts
// sizes (the path every employee/specialist creation currently takes) still
// produce concrete CPU/memory/pids limits rather than an unbounded container.
func TestResourceLimitsAppliesDefaults(t *testing.T) {
	res := resourceLimits(0, 0)

	wantCPU := int64(defaultCPUCores) * 1_000_000_000
	if res.NanoCPUs != wantCPU {
		t.Errorf("NanoCPUs = %d, want %d", res.NanoCPUs, wantCPU)
	}
	wantMem := int64(defaultMemoryGB) * bytesPerGiB
	if res.Memory != wantMem {
		t.Errorf("Memory = %d, want %d", res.Memory, wantMem)
	}
	if res.PidsLimit == nil {
		t.Fatal("PidsLimit = nil, want a bounded default")
	}
	if *res.PidsLimit != defaultPidsLimit {
		t.Errorf("PidsLimit = %d, want %d", *res.PidsLimit, defaultPidsLimit)
	}
}

// TestResourceLimitsHonorsExplicitSizes verifies per-plan sizes override the
// defaults while pids stays bounded.
func TestResourceLimitsHonorsExplicitSizes(t *testing.T) {
	res := resourceLimits(8, 16)

	if want := int64(8) * 1_000_000_000; res.NanoCPUs != want {
		t.Errorf("NanoCPUs = %d, want %d", res.NanoCPUs, want)
	}
	if want := int64(16) * bytesPerGiB; res.Memory != want {
		t.Errorf("Memory = %d, want %d", res.Memory, want)
	}
	if res.PidsLimit == nil || *res.PidsLimit != defaultPidsLimit {
		t.Errorf("PidsLimit = %v, want %d", res.PidsLimit, defaultPidsLimit)
	}
}
