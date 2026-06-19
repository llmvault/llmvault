package docker

import "testing"

// Zero-valued CreateSandboxOpts sizes still produce concrete CPU/memory/pids
// limits rather than an unbounded container.
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

// Per-plan sizes override the defaults while pids stays bounded.
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

func TestStorageOpt(t *testing.T) {
	t.Run("zero disk returns nil", func(t *testing.T) {
		if got := storageOpt(0); got != nil {
			t.Errorf("storageOpt(0) = %v, want nil", got)
		}
	})
	t.Run("negative disk returns nil", func(t *testing.T) {
		if got := storageOpt(-1); got != nil {
			t.Errorf("storageOpt(-1) = %v, want nil", got)
		}
	})
	t.Run("positive disk returns size opt", func(t *testing.T) {
		got := storageOpt(10)
		if got == nil {
			t.Fatal("storageOpt(10) = nil, want non-nil")
		}
		if got["size"] != "10G" {
			t.Errorf("storageOpt(10)[\"size\"] = %q, want %q", got["size"], "10G")
		}
	})
	t.Run("large disk value", func(t *testing.T) {
		got := storageOpt(200)
		if got == nil {
			t.Fatal("storageOpt(200) = nil, want non-nil")
		}
		if got["size"] != "200G" {
			t.Errorf("storageOpt(200)[\"size\"] = %q, want %q", got["size"], "200G")
		}
	})
}
