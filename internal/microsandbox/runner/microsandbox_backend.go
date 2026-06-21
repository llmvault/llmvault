package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/keyedlock"
	"github.com/usehivy/hivy/internal/microsandbox/config"
)

type MicrosandboxBackend struct {
	mu            sync.Mutex
	ports         map[string]map[int]int
	sandboxes     map[string]sandboxState
	allocator     *portAllocator
	imageRegistry string
	lifecycle     keyedlock.Locker
}

type sandboxState struct {
	ID     string
	Name   string
	Status string
	Labels map[string]string
	Ports  map[int]int
}

const (
	hivyManagedLabel              = "hivy_managed"
	sandboxIDLabel                = "sandbox_id"
	volumePurposeLabel            = "purpose"
	workspaceVolumePurpose        = "workspace"
	workspaceMountPath            = "/workspace"
	defaultSandboxDiskGB          = 40
	minWorkspaceVolumeMiB  uint32 = 1024
	minRootOverlayMiB      uint32 = 4 * 1024
	maxRootOverlayMiB      uint32 = 16 * 1024
	sandboxDataRootPath           = "/workspace/.hivy"
	sandboxTmpPath                = sandboxDataRootPath + "/tmp"
	dockerDataRootPath            = sandboxDataRootPath + "/docker"
)

func NewMicrosandboxBackend(ctx context.Context, cfg config.Config) (*MicrosandboxBackend, error) {
	if err := microsandbox.EnsureInstalled(ctx); err != nil {
		return nil, fmt.Errorf("microsandbox install check: %w", err)
	}
	allocator, err := newPortAllocator(cfg.RunnerPreviewPortRangeStart, cfg.RunnerPreviewPortRangeEnd)
	if err != nil {
		return nil, err
	}
	return &MicrosandboxBackend{
		ports:         map[string]map[int]int{},
		sandboxes:     map[string]sandboxState{},
		allocator:     allocator,
		imageRegistry: cfg.ImageRegistry,
	}, nil
}

func (m *MicrosandboxBackend) Reconcile(ctx context.Context) (*ReconcileReport, error) {
	handles, err := microsandbox.ListSandboxes(ctx)
	if err != nil {
		return nil, err
	}

	recoveredSandboxes := make(map[string]sandboxState)
	recoveredPorts := make(map[string]map[int]int)
	recoveredHostPorts := []int{}
	report := &ReconcileReport{}
	for _, handle := range handles {
		state, ok := recoverSandboxState(handle.Name(), string(handle.Status()), handle.ConfigJSON())
		if !ok {
			report.Skipped++
			continue
		}
		if state.Status != "running" {
			actual := m.actualState(ctx, state.ID)
			if actual.infrastructureRunning() {
				state.Status = "running"
			}
		}
		recoveredSandboxes[state.ID] = state
		if len(state.Ports) > 0 {
			recoveredPorts[state.ID] = cloneIntMap(state.Ports)
			for _, hostPort := range state.Ports {
				recoveredHostPorts = append(recoveredHostPorts, hostPort)
			}
			report.Ports += len(state.Ports)
		}
		report.Sandboxes++
	}

	m.mu.Lock()
	m.sandboxes = recoveredSandboxes
	m.ports = recoveredPorts
	m.mu.Unlock()
	m.allocator.resetWith(recoveredHostPorts)

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
	unlock := m.lifecycle.Lock(req.ID)
	defer unlock()

	previewPorts := uniquePorts(req.PreviewPorts)
	hostPorts, err := m.allocator.reserve(len(previewPorts))
	if err != nil {
		return nil, err
	}
	releaseReserved := true
	defer func() {
		if releaseReserved {
			m.allocator.release(hostPorts)
		}
	}()

	bindings := make([]PortBinding, 0, len(previewPorts))
	msbBindings := make([]microsandbox.PortBinding, 0, len(previewPorts))
	for i, guest := range previewPorts {
		host := hostPorts[i]
		hostPort, err := toUint16Port("host port", host)
		if err != nil {
			return nil, err
		}
		guestPort, err := toUint16Port("guest port", guest)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, PortBinding{GuestPort: guest, HostPort: host})
		msbBindings = append(msbBindings, microsandbox.PortBinding{
			Bind:      "0.0.0.0",
			HostPort:  hostPort,
			GuestPort: guestPort,
			Protocol:  microsandbox.PortProtocolTCP,
		})
	}

	workspaceVolName := workspaceVolumeName(req.ID)
	rootOverlayMiB, workspaceVolumeMiB := sandboxVolumeSizesMiB(req.DiskGB)
	if err := ensureVolume(ctx, workspaceVolName,
		microsandbox.WithVolumeKind(microsandbox.VolumeKindDisk),
		microsandbox.WithVolumeSize(workspaceVolumeMiB),
		microsandbox.WithVolumeLabels(volumeLabels(req.ID, workspaceVolumePurpose)),
	); err != nil {
		return nil, err
	}
	cpu, err := positiveUint8("cpu", req.CPU)
	if err != nil {
		_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), workspaceVolName)
		return nil, err
	}
	memoryMB, err := positiveUint32("memory_mb", req.MemoryMB)
	if err != nil {
		_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), workspaceVolName)
		return nil, err
	}
	env := sandboxEnvWithStorageDefaults(req.Env)

	opts := []microsandbox.SandboxOption{
		microsandbox.WithCPUs(cpu),
		microsandbox.WithMemory(memoryMB),
		microsandbox.WithOCIUpperSize(rootOverlayMiB),
		microsandbox.WithEnv(env),
		microsandbox.WithLabels(hivyLabels(req.ID, req.Labels)),
		microsandbox.WithPortBindings(msbBindings...),
		microsandbox.WithDetached(),
		microsandbox.WithMounts(map[string]microsandbox.MountConfig{
			workspaceMountPath: microsandbox.Mount.Named(workspaceVolName, microsandbox.MountOptions{}),
		}),
		microsandbox.WithImage(req.ImageRef),
	}
	if req.Init != nil && req.Init.Cmd != "" {
		opts = append(opts, microsandbox.WithInit(microsandbox.Init.Cmd(req.Init.Cmd, microsandbox.InitOptions{
			Args: req.Init.Args,
			Env:  req.Init.Env,
		})))
	}
	sb, err := microsandbox.CreateSandbox(ctx, req.ID, opts...)
	if err != nil {
		_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), workspaceVolName)
		return nil, err
	}
	if err := sb.Detach(ctx); err != nil {
		return nil, err
	}
	if err := m.ensureDockerDaemon(ctx, req.ID); err != nil {
		_ = m.deleteSandboxLocked(context.WithoutCancel(ctx), req.ID)
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
	releaseReserved = false
	return &CreateSandboxResponse{ID: req.ID, Ports: bindings}, nil
}

func (m *MicrosandboxBackend) StartSandbox(ctx context.Context, sandboxID string) error {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	return m.startSandboxLocked(ctx, sandboxID)
}

func (m *MicrosandboxBackend) StopSandbox(ctx context.Context, sandboxID string) error {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	return m.stopSandboxLocked(ctx, sandboxID)
}

func (m *MicrosandboxBackend) EnsureReady(ctx context.Context, sandboxID string, req EnsureReadyRequest) (*EnsureReadyResponse, error) {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	if err := m.startSandboxLocked(ctx, sandboxID); err != nil {
		return nil, err
	}
	hostPort := m.hostPortForGuest(sandboxID, req.GuestPort)
	if hostPort == 0 {
		return nil, fmt.Errorf("guest port %d is not published for sandbox %s", req.GuestPort, sandboxID)
	}
	timeout := startVerifyTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := waitForCondition(readyCtx, timeout, func() (bool, string) {
		return tcpPortOpen(hostPort, actualProbeTimeout), fmt.Sprintf("host_port=%d port_open=false", hostPort)
	}); err != nil {
		return nil, err
	}
	if req.HealthCheck != nil {
		if err := waitForHTTPHealthCheck(readyCtx, sandboxID, req.GuestPort, hostPort, *req.HealthCheck); err != nil {
			return nil, err
		}
	}
	m.setSandboxStatus(sandboxID, "running")
	return &EnsureReadyResponse{Status: "running", HostPort: hostPort}, nil
}

func (m *MicrosandboxBackend) DeleteSandbox(ctx context.Context, sandboxID string) error {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()
	return m.deleteSandboxLocked(ctx, sandboxID)
}
