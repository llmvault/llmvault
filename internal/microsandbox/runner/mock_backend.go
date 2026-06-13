package runner

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
)

type MockBackend struct {
	mu        sync.Mutex
	sandboxes map[string]*CreateSandboxResponse
}

func NewMockBackend() *MockBackend {
	return &MockBackend{sandboxes: map[string]*CreateSandboxResponse{}}
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
	ports := make([]PortBinding, 0, len(req.PreviewPorts))
	for _, p := range req.PreviewPorts {
		hostPort, err := freePort()
		if err != nil {
			return nil, err
		}
		ports = append(ports, PortBinding{GuestPort: p, HostPort: hostPort})
	}
	resp := &CreateSandboxResponse{ID: req.ID, Ports: ports}
	m.sandboxes[req.ID] = resp
	return resp, nil
}

func (m *MockBackend) StartSandbox(context.Context, string) error { return nil }
func (m *MockBackend) StopSandbox(context.Context, string) error  { return nil }
func (m *MockBackend) DeleteSandbox(_ context.Context, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *MockBackend) CreateSnapshot(_ context.Context, req CreateSnapshotRequest) (*CreateSnapshotResponse, error) {
	return &CreateSnapshotResponse{ID: req.ID, ArtifactURL: "mock://" + req.ID, Logs: "mock snapshot built\n"}, nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

var _ = http.MethodGet
