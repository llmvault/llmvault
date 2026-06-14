package runner

import (
	"fmt"
	"net"
	"testing"

	"github.com/usehivy/hivy/internal/microsandbox/api"
)

func TestPortAllocatorReservesDefaultSandboxPortsInsideRange(t *testing.T) {
	start, end := freePreviewPortWindow(t, len(api.DefaultPreviewPorts()))
	allocator, err := newPortAllocator(start, end)
	if err != nil {
		t.Fatal(err)
	}

	ports, err := allocator.reserve(len(api.DefaultPreviewPorts()))
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 5 {
		t.Fatalf("reserved %d ports, want 5", len(ports))
	}
	for _, port := range ports {
		if port < start || port > end {
			t.Fatalf("reserved port %d outside range %d-%d", port, start, end)
		}
	}
}

func TestPortAllocatorHandlesCollisions(t *testing.T) {
	start, end := freePreviewPortWindow(t, 3)
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", start))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	allocator, err := newPortAllocator(start, end)
	if err != nil {
		t.Fatal(err)
	}
	ports, err := allocator.reserve(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range ports {
		if port == start {
			t.Fatalf("allocator reserved externally occupied port %d", port)
		}
	}
}

func TestPortAllocatorReleasesPartialReservationOnExhaustion(t *testing.T) {
	start, end := freePreviewPortWindow(t, 2)
	allocator, err := newPortAllocator(start, end)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := allocator.reserve(3); err == nil {
		t.Fatal("expected exhaustion error")
	}
	ports, err := allocator.reserve(2)
	if err != nil {
		t.Fatalf("expected partial reservation to be released: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("reserved %d ports, want 2", len(ports))
	}
}

func TestPortAllocatorReleaseAllowsReuse(t *testing.T) {
	start, end := freePreviewPortWindow(t, 2)
	allocator, err := newPortAllocator(start, end)
	if err != nil {
		t.Fatal(err)
	}

	first, err := allocator.reserve(2)
	if err != nil {
		t.Fatal(err)
	}
	allocator.release(first)
	second, err := allocator.reserve(2)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("released ports were not reused: first=%v second=%v", first, second)
		}
	}
}

func TestPreviewPortCapacityHelper(t *testing.T) {
	got := newDefaultPortAllocator().capacity(len(api.DefaultPreviewPorts()))
	if got != 6200 {
		t.Fatalf("capacity = %d, want 6200", got)
	}
}

func freePreviewPortWindow(t *testing.T, count int) (int, int) {
	t.Helper()
	for start := api.DefaultPreviewHostPortRangeStart; start+count-1 <= api.DefaultPreviewHostPortRangeEnd; start++ {
		listeners := make([]net.Listener, 0, count)
		ok := true
		for port := start; port < start+count; port++ {
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				ok = false
				break
			}
			listeners = append(listeners, ln)
		}
		for _, ln := range listeners {
			_ = ln.Close()
		}
		if ok {
			return start, start + count - 1
		}
	}
	t.Fatalf("no free preview port window of %d ports in %d-%d", count, api.DefaultPreviewHostPortRangeStart, api.DefaultPreviewHostPortRangeEnd)
	return 0, 0
}
