package runner

import (
	"context"
	"fmt"
	"sync"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/storage"
)

type MicrosandboxBackend struct {
	mu               sync.Mutex
	snapshotImportMu sync.Mutex
	ports            map[string]map[int]int
	sandboxes        map[string]sandboxState
	store            *storage.SnapshotStore
	allocator        *portAllocator
}

type sandboxState struct {
	ID     string
	Name   string
	Status string
	Labels map[string]string
	Ports  map[int]int
}

const (
	hivyManagedLabel               = "hivy_managed"
	sandboxIDLabel                 = "sandbox_id"
	volumePurposeLabel             = "purpose"
	workspaceVolumePurpose         = "workspace"
	dockerDataVolumePurpose        = "docker_data"
	dockerDataMountPath            = "/var/lib/docker"
	workspaceMountPath             = "/workspace"
	defaultSandboxDiskGB           = 40
	minWorkspaceVolumeMiB   uint32 = 1024
	maxDockerDataVolumeMiB  uint32 = 20 * 1024
)

func NewMicrosandboxBackend(ctx context.Context, cfg config.Config) (*MicrosandboxBackend, error) {
	if err := microsandbox.EnsureInstalled(ctx); err != nil {
		return nil, fmt.Errorf("microsandbox install check: %w", err)
	}
	store, err := storage.NewSnapshotStore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("snapshot store: %w", err)
	}
	allocator, err := newPortAllocator(cfg.RunnerPreviewPortRangeStart, cfg.RunnerPreviewPortRangeEnd)
	if err != nil {
		return nil, err
	}
	return &MicrosandboxBackend{
		ports:     map[string]map[int]int{},
		sandboxes: map[string]sandboxState{},
		store:     store,
		allocator: allocator,
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
		bindings = append(bindings, PortBinding{GuestPort: guest, HostPort: host})
		msbBindings = append(msbBindings, microsandbox.PortBinding{
			Bind:      "0.0.0.0",
			HostPort:  uint16(host),
			GuestPort: uint16(guest),
			Protocol:  microsandbox.PortProtocolTCP,
		})
	}

	workspaceVolName := workspaceVolumeName(req.ID)
	dockerVolName := dockerDataVolumeName(req.ID)
	workspaceVolumeMiB, dockerDataVolumeMiB := sandboxVolumeSizesMiB(req.DiskGB)
	if err := ensureVolume(ctx, workspaceVolName,
		microsandbox.WithVolumeQuota(workspaceVolumeMiB),
		microsandbox.WithVolumeLabels(volumeLabels(req.ID, workspaceVolumePurpose)),
	); err != nil {
		return nil, err
	}
	if err := ensureVolume(ctx, dockerVolName,
		microsandbox.WithVolumeKind(microsandbox.VolumeKindDisk),
		microsandbox.WithVolumeSize(dockerDataVolumeMiB),
		microsandbox.WithVolumeLabels(volumeLabels(req.ID, dockerDataVolumePurpose)),
	); err != nil {
		_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), workspaceVolName)
		return nil, err
	}
	if req.SnapshotID != "" {
		if err := m.ensureSnapshotAvailable(ctx, req); err != nil {
			_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), workspaceVolName)
			_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), dockerVolName)
			return nil, err
		}
	}
	opts := []microsandbox.SandboxOption{
		microsandbox.WithCPUs(uint8(req.CPU)),
		microsandbox.WithMemory(uint32(req.MemoryMB)),
		microsandbox.WithEnv(req.Env),
		microsandbox.WithLabels(hivyLabels(req.ID, req.Labels)),
		microsandbox.WithPortBindings(msbBindings...),
		microsandbox.WithDetached(),
		microsandbox.WithMounts(map[string]microsandbox.MountConfig{
			workspaceMountPath:  microsandbox.Mount.Named(workspaceVolName, microsandbox.MountOptions{}),
			dockerDataMountPath: microsandbox.Mount.Named(dockerVolName, microsandbox.MountOptions{}),
		}),
	}
	if req.SnapshotID != "" {
		opts = append(opts, microsandbox.WithSnapshot(req.SnapshotID))
	} else {
		opts = append(opts, microsandbox.WithImage(req.ImageRef))
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
		_ = microsandbox.RemoveVolume(context.WithoutCancel(ctx), dockerVolName)
		return nil, err
	}
	if err := sb.Detach(ctx); err != nil {
		return nil, err
	}
	if err := m.ensureDockerDaemon(ctx, req.ID); err != nil {
		_ = m.DeleteSandbox(context.WithoutCancel(ctx), req.ID)
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
	sb, err := microsandbox.StartSandboxDetached(ctx, sandboxID)
	if err != nil {
		return err
	}
	if err := sb.Detach(ctx); err != nil {
		return err
	}
	if err := m.ensureDockerDaemon(ctx, sandboxID); err != nil {
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
	_ = microsandbox.RemoveVolume(ctx, workspaceVolumeName(sandboxID))
	_ = microsandbox.RemoveVolume(ctx, dockerDataVolumeName(sandboxID))
	m.mu.Lock()
	released := mapValues(m.ports[sandboxID])
	delete(m.ports, sandboxID)
	delete(m.sandboxes, sandboxID)
	m.mu.Unlock()
	m.allocator.release(released)
	return nil
}
