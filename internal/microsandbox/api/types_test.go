package api

import "testing"

func TestDefaultPreviewPortsIncludesCommonDevPorts(t *testing.T) {
	ports := DefaultPreviewPorts()
	if len(ports) != 5 {
		t.Fatalf("default preview ports length = %d, want 5", len(ports))
	}
	required := []int{3000, 5173, 7080, 8000, 8080}
	seen := map[int]bool{}
	for _, port := range ports {
		if seen[port] {
			t.Fatalf("duplicate default preview port %d", port)
		}
		seen[port] = true
	}
	for _, port := range required {
		if !seen[port] {
			t.Fatalf("default preview ports missing %d: %v", port, ports)
		}
	}
}

func TestMicroSizeIsSmallestTier(t *testing.T) {
	micro, ok := Sizes["micro"]
	if !ok {
		t.Fatal("micro size missing from Sizes table")
	}
	want := Size{CPU: 1, MemoryMB: 256, DiskGB: 5}
	if micro != want {
		t.Fatalf("micro = %+v, want %+v", micro, want)
	}
	// The authoritative micro memory (256 MB) is below nano (1024 MB).
	if micro.MemoryMB >= Sizes["nano"].MemoryMB {
		t.Fatalf("micro memory %d should be below nano %d", micro.MemoryMB, Sizes["nano"].MemoryMB)
	}
}

func TestPreviewPortCapacityForDefaultRange(t *testing.T) {
	got := PreviewPortCapacity(DefaultPreviewHostPortRangeStart, DefaultPreviewHostPortRangeEnd, len(DefaultPreviewPorts()))
	if got != 6200 {
		t.Fatalf("preview port capacity = %d, want 6200", got)
	}
}
