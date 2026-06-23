package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type MockBackend struct {
	mu        sync.Mutex
	sandboxes map[string]*CreateSandboxResponse
	allocator *portAllocator
}

func NewMockBackend() *MockBackend {
	return &MockBackend{sandboxes: map[string]*CreateSandboxResponse{}, allocator: newDefaultPortAllocator()}
}

func (m *MockBackend) Reconcile(context.Context) (*ReconcileReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ports := 0
	for _, sandbox := range m.sandboxes {
		ports += len(sandbox.Ports)
	}
	return &ReconcileReport{Sandboxes: len(m.sandboxes), Ports: ports}, nil
}

func (m *MockBackend) Status(context.Context) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{"running_sandboxes": len(m.sandboxes)}, nil
}

func (m *MockBackend) CreateSandbox(_ context.Context, req CreateSandboxRequest) (*CreateSandboxResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	previewPorts := uniquePorts(req.PreviewPorts)
	hostPorts, err := m.allocator.reserve(len(previewPorts))
	if err != nil {
		return nil, err
	}
	ports := make([]PortBinding, 0, len(previewPorts))
	for i, p := range previewPorts {
		ports = append(ports, PortBinding{GuestPort: p, HostPort: hostPorts[i]})
	}
	resp := &CreateSandboxResponse{ID: req.ID, Ports: ports}
	m.sandboxes[req.ID] = resp
	return resp, nil
}

func (m *MockBackend) UpgradeSandbox(_ context.Context, sandboxID string, req UpgradeSandboxRequest) (*UpgradeSandboxResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ports := append([]PortBinding(nil), req.PortBindings...)
	if len(ports) == 0 {
		if existing := m.sandboxes[sandboxID]; existing != nil {
			ports = append(ports, existing.Ports...)
		}
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("sandbox %s has no existing port bindings", sandboxID)
	}
	resp := &CreateSandboxResponse{ID: sandboxID, Ports: ports}
	m.sandboxes[sandboxID] = resp
	return &UpgradeSandboxResponse{ID: sandboxID, Status: "running", Ports: ports}, nil
}

func (m *MockBackend) StartSandbox(context.Context, string) error { return nil }
func (m *MockBackend) StopSandbox(context.Context, string) error  { return nil }
func (m *MockBackend) EnsureReady(_ context.Context, sandboxID string, req EnsureReadyRequest) (*EnsureReadyResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hostPort := 1
	if sandbox := m.sandboxes[sandboxID]; sandbox != nil {
		for _, port := range sandbox.Ports {
			if port.GuestPort == req.GuestPort {
				hostPort = port.HostPort
				break
			}
		}
	}
	return &EnsureReadyResponse{Status: "running", HostPort: hostPort}, nil
}
func (m *MockBackend) Connections(_ context.Context, sandboxID string) (*ConnectionsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	resp := &ConnectionsResponse{SandboxID: sandboxID, ByGuestPort: map[int]int{}, ByHostPort: map[int]int{}}
	if sandbox := m.sandboxes[sandboxID]; sandbox != nil {
		for _, port := range sandbox.Ports {
			resp.ByGuestPort[port.GuestPort] = 0
			resp.ByHostPort[port.HostPort] = 0
		}
	}
	return resp, nil
}
func (m *MockBackend) DeleteSandbox(_ context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sandbox := m.sandboxes[sandboxID]; sandbox != nil {
		hostPorts := make([]int, 0, len(sandbox.Ports))
		for _, port := range sandbox.Ports {
			hostPorts = append(hostPorts, port.HostPort)
		}
		m.allocator.release(hostPorts)
	}
	delete(m.sandboxes, sandboxID)
	return nil
}

func (m *MockBackend) Exec(_ context.Context, _ string, command string, _ int) (*ExecResponse, error) {
	return &ExecResponse{Stdout: "mock executed: " + command + "\n", ExitCode: 0}, nil
}

func (m *MockBackend) Logs(_ context.Context, sandboxID string, w io.Writer) error {
	_, err := fmt.Fprintf(w, "mock logs for %s\n", sandboxID)
	return err
}

func (m *MockBackend) Proxy(context.Context, string, int, io.Writer, io.Reader) error { return nil }

func (m *MockBackend) ProxyURL(_ context.Context, sandboxID string, guestPort int) (string, error) {
	return "http://127.0.0.1:1", nil
}

func (m *MockBackend) CreateTemplate(_ context.Context, req CreateTemplateRequest, onEvent func(TemplateBuildEvent)) (*CreateTemplateResponse, error) {
	if onEvent != nil {
		onEvent(TemplateBuildEvent{Type: "log", Message: "mock template built"})
	}
	return &CreateTemplateResponse{
		ID:                  req.ID,
		ImageRef:            "mock-registry/images/" + req.OrgID + "/" + req.ID + "@sha256:mock",
		ImageDigest:         "sha256:mock",
		ValidationSandboxID: "mock-validation",
		Logs:                "mock template built\n",
	}, nil
}

var _ = http.MethodGet
