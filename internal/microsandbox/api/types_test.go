package api

import "testing"

func TestDefaultPreviewPortsIncludesCommonDevPorts(t *testing.T) {
	ports := DefaultPreviewPorts()
	if len(ports) != 30 {
		t.Fatalf("default preview ports length = %d, want 30", len(ports))
	}
	required := []int{3000, 4173, 4200, 4321, 5000, 5173, 6006, 8000, 8080, 8501, 9000}
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
