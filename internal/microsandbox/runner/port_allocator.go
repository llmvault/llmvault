package runner

import (
	"fmt"
	"net"
	"sync"

	"github.com/usehivy/hivy/internal/microsandbox/api"
)

type portAllocator struct {
	mu       sync.Mutex
	start    int
	end      int
	reserved map[int]struct{}
}

func newPortAllocator(start, end int) (*portAllocator, error) {
	if start <= 0 || end < start || end > 65535 {
		return nil, fmt.Errorf("invalid preview host port range %d-%d", start, end)
	}
	return &portAllocator{
		start:    start,
		end:      end,
		reserved: map[int]struct{}{},
	}, nil
}

func newDefaultPortAllocator() *portAllocator {
	allocator, err := newPortAllocator(api.DefaultPreviewHostPortRangeStart, api.DefaultPreviewHostPortRangeEnd)
	if err != nil {
		panic(err)
	}
	return allocator
}

func (a *portAllocator) reserve(count int) ([]int, error) {
	if count <= 0 {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	ports := make([]int, 0, count)
	for port := a.start; port <= a.end && len(ports) < count; port++ {
		if _, ok := a.reserved[port]; ok {
			continue
		}
		if !tcpPortAvailable(port) {
			continue
		}
		a.reserved[port] = struct{}{}
		ports = append(ports, port)
	}
	if len(ports) != count {
		for _, port := range ports {
			delete(a.reserved, port)
		}
		return nil, fmt.Errorf("insufficient preview host ports available: requested %d, reserved %d, range %d-%d", count, len(ports), a.start, a.end)
	}
	return ports, nil
}

func (a *portAllocator) release(ports []int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, port := range ports {
		delete(a.reserved, port)
	}
}

func (a *portAllocator) resetWith(ports []int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reserved = map[int]struct{}{}
	for _, port := range ports {
		if port >= a.start && port <= a.end {
			a.reserved[port] = struct{}{}
		}
	}
}

func (a *portAllocator) capacity(portsPerSandbox int) int {
	return api.PreviewPortCapacity(a.start, a.end, portsPerSandbox)
}

func tcpPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
