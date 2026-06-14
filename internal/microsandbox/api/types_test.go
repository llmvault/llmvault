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

func TestPreviewPortCapacityForDefaultRange(t *testing.T) {
	got := PreviewPortCapacity(DefaultPreviewHostPortRangeStart, DefaultPreviewHostPortRangeEnd, len(DefaultPreviewPorts()))
	if got != 6200 {
		t.Fatalf("preview port capacity = %d, want 6200", got)
	}
}
