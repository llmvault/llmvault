package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/storage"
)

type MicrosandboxBackend struct {
	mu        sync.Mutex
	ports     map[string]map[int]int
	sandboxes map[string]sandboxState
	store     *storage.SnapshotStore
}

type sandboxState struct {
	ID     string
	Name   string
	Status string
	Labels map[string]string
	Ports  map[int]int
}

const (
	hivyManagedLabel = "hivy_managed"
	sandboxIDLabel   = "sandbox_id"
)

func NewMicrosandboxBackend(ctx context.Context, cfg config.Config) (*MicrosandboxBackend, error) {
	if err := microsandbox.EnsureInstalled(ctx); err != nil {
		return nil, fmt.Errorf("microsandbox install check: %w", err)
	}
	store, err := storage.NewSnapshotStore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("snapshot store: %w", err)
	}
	return &MicrosandboxBackend{
		ports:     map[string]map[int]int{},
		sandboxes: map[string]sandboxState{},
		store:     store,
	}, nil
}

func (m *MicrosandboxBackend) Reconcile(ctx context.Context) (*ReconcileReport, error) {
	handles, err := microsandbox.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}

	recoveredSandboxes := make(map[string]sandboxState)
	recoveredPorts := make(map[string]map[int]int)
	report := &ReconcileReport{}
	for _, handle := range handles {
		state, ok := recoverSandboxState(handle.Name(), string(handle.Status()), handle.ConfigJSON())
		if !ok {
			report.Skipped++
			continue
		}
		recoveredSandboxes[state.ID] = state
		if len(state.Ports) > 0 {
			recoveredPorts[state.ID] = cloneIntMap(state.Ports)
			report.Ports += len(state.Ports)
		}
		report.Sandboxes++
	}

	m.mu.Lock()
	m.sandboxes = recoveredSandboxes
	m.ports = recoveredPorts
	m.mu.Unlock()

	return report, nil
}

func (m *MicrosandboxBackend) Status(ctx context.Context) (map[string]any, error) {
	handles, err := microsandbox.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	knownSandboxes := len(m.sandboxes)
	publishedPorts := 0
	for _, ports := range m.ports {
		publishedPorts += len(ports)
	}
	m.mu.Unlock()
	return map[string]any{
		"running_sandboxes": len(handles),
		"known_sandboxes":   knownSandboxes,
		"published_ports":   publishedPorts,
	}, nil
}

func (m *MicrosandboxBackend) CreateSandbox(ctx context.Context, req CreateSandboxRequest) (*CreateSandboxResponse, error) {
	hostToGuest := map[uint16]uint16{}
	bindings := make([]PortBinding, 0, len(req.PreviewPorts))
	for _, guest := range req.PreviewPorts {
		host, err := freePort()
		if err != nil {
			return nil, err
		}
		hostToGuest[uint16(host)] = uint16(guest)
		bindings = append(bindings, PortBinding{GuestPort: guest, HostPort: host})
	}

	volName := "hivy-" + req.ID
	_, _ = microsandbox.CreateVolume(ctx, volName,
		microsandbox.WithVolumeQuota(uint32(req.DiskGB*1024)),
		microsandbox.WithVolumeLabels(map[string]string{"sandbox_id": req.ID}),
	)
	opts := []microsandbox.SandboxOption{
		microsandbox.WithCPUs(uint8(req.CPU)),
		microsandbox.WithMemory(uint32(req.MemoryMB)),
		microsandbox.WithEnv(req.Env),
		microsandbox.WithLabels(hivyLabels(req.ID, req.Labels)),
		microsandbox.WithPorts(hostToGuest),
		microsandbox.WithDetached(),
		microsandbox.WithMounts(map[string]microsandbox.MountConfig{
			"/workspace": microsandbox.Mount.Named(volName, microsandbox.MountOptions{}),
		}),
	}
	if req.SnapshotID != "" {
		opts = append(opts, microsandbox.WithSnapshot(req.SnapshotID))
	} else {
		opts = append(opts, microsandbox.WithImage(req.ImageRef))
	}
	sb, err := microsandbox.CreateSandbox(ctx, req.ID, opts...)
	if err != nil {
		return nil, err
	}
	if err := sb.Detach(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.ports[req.ID] = map[int]int{}
	for _, binding := range bindings {
		m.ports[req.ID][binding.GuestPort] = binding.HostPort
	}
	m.sandboxes[req.ID] = sandboxState{
		ID:     req.ID,
		Name:   req.Name,
		Status: "running",
		Labels: hivyLabels(req.ID, req.Labels),
		Ports:  cloneIntMap(m.ports[req.ID]),
	}
	m.mu.Unlock()
	return &CreateSandboxResponse{ID: req.ID, Ports: bindings}, nil
}

func (m *MicrosandboxBackend) StartSandbox(ctx context.Context, sandboxID string) error {
	sb, err := microsandbox.StartSandboxDetached(ctx, sandboxID)
	if err != nil {
		return err
	}
	if err := sb.Detach(ctx); err != nil {
		return err
	}
	m.setSandboxStatus(sandboxID, "running")
	return nil
}

func (m *MicrosandboxBackend) StopSandbox(ctx context.Context, sandboxID string) error {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err != nil {
		return err
	}
	if err := handle.Stop(ctx); err != nil {
		return err
	}
	m.setSandboxStatus(sandboxID, "stopped")
	return nil
}

func (m *MicrosandboxBackend) DeleteSandbox(ctx context.Context, sandboxID string) error {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err == nil {
		_ = handle.Stop(ctx)
		_ = handle.Remove(ctx)
	}
	_ = microsandbox.RemoveVolume(ctx, "hivy-"+sandboxID)
	m.mu.Lock()
	delete(m.ports, sandboxID)
	delete(m.sandboxes, sandboxID)
	m.mu.Unlock()
	return nil
}

func (m *MicrosandboxBackend) Exec(ctx context.Context, sandboxID, command string, timeoutSeconds int) (*ExecResponse, error) {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err != nil {
		return nil, err
	}
	sb, err := handle.Connect(ctx)
	if err != nil {
		return nil, err
	}
	defer sb.Close()
	opts := []microsandbox.ExecOption{}
	if timeoutSeconds > 0 {
		opts = append(opts, microsandbox.WithExecTimeout(time.Duration(timeoutSeconds)*time.Second))
	}
	out, err := sb.Shell(ctx, command, opts...)
	if err != nil {
		return nil, err
	}
	return &ExecResponse{Stdout: out.Stdout(), Stderr: out.Stderr(), ExitCode: out.ExitCode()}, nil
}

func (m *MicrosandboxBackend) Logs(ctx context.Context, sandboxID string, w io.Writer) error {
	resp, err := m.Exec(ctx, sandboxID, "tail -200 /tmp/*.log 2>/dev/null || true", 10)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, resp.Stdout+resp.Stderr)
	return err
}

func (m *MicrosandboxBackend) Proxy(context.Context, string, int, io.Writer, io.Reader) error {
	return fmt.Errorf("direct proxy is handled by ProxyURL")
}

func (m *MicrosandboxBackend) ProxyURL(_ context.Context, sandboxID string, guestPort int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ports := m.ports[sandboxID]
	hostPort := ports[guestPort]
	if hostPort == 0 {
		return "", fmt.Errorf("port %d is not published for sandbox %s", guestPort, sandboxID)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", hostPort), nil
}

func (m *MicrosandboxBackend) CreateSnapshot(ctx context.Context, req CreateSnapshotRequest) (*CreateSnapshotResponse, error) {
	sbReq := CreateSandboxRequest{
		ID: req.ID, Name: req.Name, ImageRef: req.BaseImageRef, CPU: req.CPU,
		MemoryMB: req.MemoryMB, DiskGB: req.DiskGB, Env: req.Env,
	}
	if _, err := m.CreateSandbox(ctx, sbReq); err != nil {
		return nil, err
	}
	defer func() {
		_ = m.DeleteSandbox(context.WithoutCancel(ctx), req.ID)
	}()
	var logs strings.Builder
	for _, cmd := range req.Commands {
		out, err := m.Exec(ctx, req.ID, cmd, 0)
		if out != nil {
			logs.WriteString(out.Stdout)
			logs.WriteString(out.Stderr)
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot command failed %q: %w", cmd, err)
		}
		if out != nil && out.ExitCode != 0 {
			return nil, fmt.Errorf("snapshot command failed %q with exit code %d", cmd, out.ExitCode)
		}
	}
	if err := m.StopSandbox(ctx, req.ID); err != nil {
		return nil, err
	}
	snapshot, err := microsandbox.Snapshot.Create(ctx, req.ID, microsandbox.SnapshotCreateOptions{
		Name:            req.ID,
		Force:           true,
		RecordIntegrity: true,
		Labels:          map[string]string{"snapshot_id": req.ID, "name": req.Name},
	})
	if err != nil {
		return nil, err
	}
	if _, err := snapshot.Verify(ctx); err != nil {
		return nil, err
	}
	exportDir := filepath.Join(os.TempDir(), "microsandbox-snapshots")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, err
	}
	exportPath := filepath.Join(exportDir, req.ID+".tar")
	if err := microsandbox.Snapshot.Export(ctx, snapshot.Path(), exportPath, microsandbox.SnapshotExportOptions{
		WithParents: true,
		WithImage:   true,
		PlainTar:    true,
	}); err != nil {
		return nil, err
	}
	artifactURL, err := m.store.Upload(ctx, exportPath, req.ID)
	if err != nil {
		return nil, err
	}
	return &CreateSnapshotResponse{ID: req.ID, ArtifactURL: artifactURL, Logs: logs.String()}, nil
}

type persistedSandboxConfig struct {
	Labels       map[string]string       `json:"labels"`
	Ports        json.RawMessage         `json:"ports"`
	PortBindings []persistedPortBinding  `json:"port_bindings"`
	Network      *persistedNetworkConfig `json:"network"`
}

type persistedNetworkConfig struct {
	Ports        json.RawMessage        `json:"ports"`
	PortBindings []persistedPortBinding `json:"port_bindings"`
}

type persistedPortBinding struct {
	HostPort  uint16 `json:"host_port"`
	GuestPort uint16 `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

func recoverSandboxState(name, status, configJSON string) (sandboxState, bool) {
	var cfg persistedSandboxConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return sandboxState{}, false
		}
	}

	sandboxID := cfg.Labels[sandboxIDLabel]
	if sandboxID == "" && isHivySandboxName(name) {
		sandboxID = name
	}
	if sandboxID == "" {
		return sandboxState{}, false
	}

	ports := publishedPorts(cfg)
	return sandboxState{
		ID:     sandboxID,
		Name:   name,
		Status: status,
		Labels: cloneStringMap(cfg.Labels),
		Ports:  ports,
	}, true
}

func publishedPorts(cfg persistedSandboxConfig) map[int]int {
	ports := map[int]int{}
	addPublishedPorts(ports, cfg.Ports)
	addPublishedPortBindings(ports, cfg.PortBindings)
	if cfg.Network != nil {
		addPublishedPorts(ports, cfg.Network.Ports)
		addPublishedPortBindings(ports, cfg.Network.PortBindings)
	}
	return ports
}

func addPublishedPorts(dst map[int]int, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var hostToGuest map[string]uint16
	if err := json.Unmarshal(raw, &hostToGuest); err == nil {
		addPublishedPortMap(dst, hostToGuest)
		return
	}
	var bindings []persistedPortBinding
	if err := json.Unmarshal(raw, &bindings); err == nil {
		addPublishedPortBindings(dst, bindings)
	}
}

func addPublishedPortMap(dst map[int]int, hostToGuest map[string]uint16) {
	for hostRaw, guest := range hostToGuest {
		host, err := strconv.Atoi(hostRaw)
		if err != nil || host <= 0 || guest == 0 {
			continue
		}
		dst[int(guest)] = host
	}
}

func addPublishedPortBindings(dst map[int]int, bindings []persistedPortBinding) {
	for _, binding := range bindings {
		if binding.HostPort == 0 || binding.GuestPort == 0 {
			continue
		}
		if binding.Protocol != "" && !strings.EqualFold(binding.Protocol, "tcp") {
			continue
		}
		dst[int(binding.GuestPort)] = int(binding.HostPort)
	}
}

func hivyLabels(sandboxID string, labels map[string]string) map[string]string {
	out := cloneStringMap(labels)
	out[hivyManagedLabel] = "true"
	if out[sandboxIDLabel] == "" && isHivySandboxName(sandboxID) {
		out[sandboxIDLabel] = sandboxID
	}
	return out
}

func isHivySandboxName(name string) bool {
	return strings.HasPrefix(name, "sbx_")
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneIntMap(in map[int]int) map[int]int {
	out := make(map[int]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *MicrosandboxBackend) setSandboxStatus(sandboxID, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.sandboxes[sandboxID]
	if !ok {
		return
	}
	state.Status = status
	m.sandboxes[sandboxID] = state
}
