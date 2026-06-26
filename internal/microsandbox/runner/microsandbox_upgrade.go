package runner

import (
	"context"
	"fmt"
	"sort"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

const upgradeStatusRolledBack = "rolled_back"

func (m *MicrosandboxBackend) UpgradeSandbox(ctx context.Context, sandboxID string, req UpgradeSandboxRequest) (*UpgradeSandboxResponse, error) {
	unlock := m.lifecycle.Lock(sandboxID)
	defer unlock()

	bindings, err := m.upgradePortBindings(sandboxID, req)
	if err != nil {
		return nil, err
	}
	if req.ImageRef == "" {
		return nil, fmt.Errorf("image_ref is required")
	}
	if req.PreviousImageRef == "" {
		return nil, fmt.Errorf("previous_image_ref is required")
	}

	if err := m.stopSandboxLocked(ctx, sandboxID); err != nil {
		return nil, fmt.Errorf("stop sandbox before upgrade: %w", err)
	}
	if err := m.removeSandboxObject(ctx, sandboxID); err != nil {
		return nil, fmt.Errorf("remove sandbox before upgrade: %w", err)
	}
	if err := m.createSandboxWithBindings(ctx, sandboxID, req, bindings, req.ImageRef); err != nil {
		rollbackReq := req
		if rollbackReq.Name == "" {
			rollbackReq.Name = sandboxID
		}
		rollbackErr := m.createSandboxWithBindings(ctx, sandboxID, rollbackReq, bindings, req.PreviousImageRef)
		if rollbackErr != nil {
			return nil, fmt.Errorf("upgrade create failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return &UpgradeSandboxResponse{
			ID:     sandboxID,
			Status: upgradeStatusRolledBack,
			Error:  err.Error(),
			Ports:  bindings,
		}, nil
	}
	return &UpgradeSandboxResponse{ID: sandboxID, Status: "running", Ports: bindings}, nil
}

func (m *MicrosandboxBackend) upgradePortBindings(sandboxID string, req UpgradeSandboxRequest) ([]PortBinding, error) {
	bindings := append([]PortBinding(nil), req.PortBindings...)
	if len(bindings) == 0 {
		ports := m.sandboxPorts(sandboxID)
		bindings = make([]PortBinding, 0, len(ports))
		for guest, host := range ports {
			bindings = append(bindings, PortBinding{GuestPort: guest, HostPort: host})
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].GuestPort < bindings[j].GuestPort
	})
	if len(bindings) == 0 {
		return nil, fmt.Errorf("sandbox %s has no existing port bindings", sandboxID)
	}
	if err := validateRequestedPortSet(req.PreviewPorts, bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

func validateRequestedPortSet(previewPorts []int, bindings []PortBinding) error {
	requested := uniquePorts(previewPorts)
	if len(requested) == 0 {
		return nil
	}
	bound := make(map[int]struct{}, len(bindings))
	for _, binding := range bindings {
		bound[binding.GuestPort] = struct{}{}
	}
	if len(requested) != len(bound) {
		return fmt.Errorf("preview_ports cannot change during upgrade")
	}
	for _, port := range requested {
		if _, ok := bound[port]; !ok {
			return fmt.Errorf("preview_ports cannot change during upgrade")
		}
	}
	return nil
}

func (m *MicrosandboxBackend) removeSandboxObject(ctx context.Context, sandboxID string) error {
	handle, err := microsandbox.GetSandbox(ctx, sandboxID)
	if err != nil {
		return ignoreNotFound(err)
	}
	return handle.Remove(ctx)
}

func (m *MicrosandboxBackend) createSandboxWithBindings(ctx context.Context, sandboxID string, req UpgradeSandboxRequest, bindings []PortBinding, imageRef string) error {
	workspaceVolName := workspaceVolumeName(sandboxID)
	rootOverlayMiB, workspaceVolumeMiB := sandboxVolumeSizesMiB(req.DiskGB)
	if err := ensureVolume(ctx, workspaceVolName,
		microsandbox.WithVolumeKind(microsandbox.VolumeKindDisk),
		microsandbox.WithVolumeSize(workspaceVolumeMiB),
		microsandbox.WithVolumeLabels(volumeLabels(sandboxID, workspaceVolumePurpose)),
	); err != nil {
		return err
	}
	cpu, err := positiveUint8("cpu", req.CPU)
	if err != nil {
		return err
	}
	memoryMB, err := positiveUint32("memory_mb", req.MemoryMB)
	if err != nil {
		return err
	}
	msbBindings, err := microsandboxPortBindings(bindings)
	if err != nil {
		return err
	}
	labels := hivyLabels(sandboxID, req.Labels)
	opts := []microsandbox.SandboxOption{
		microsandbox.WithCPUs(cpu),
		microsandbox.WithMemory(memoryMB),
		microsandbox.WithOCIUpperSize(rootOverlayMiB),
		microsandbox.WithEnv(sandboxEnvWithStorageDefaults(req.Env)),
		microsandbox.WithLabels(labels),
		microsandbox.WithPortBindings(msbBindings...),
		microsandbox.WithDetached(),
		microsandbox.WithPullPolicy(microsandbox.PullPolicyAlways),
		microsandbox.WithMounts(map[string]microsandbox.MountConfig{
			workspaceMountPath: microsandbox.Mount.Named(workspaceVolName, microsandbox.MountOptions{}),
		}),
		microsandbox.WithImage(imageRef),
	}
	if initOpt, ok := sandboxInitOption(req.Init); ok {
		opts = append(opts, initOpt)
	}
	sb, err := microsandbox.CreateSandbox(ctx, sandboxID, opts...)
	if err != nil {
		return err
	}
	if err := sb.Detach(ctx); err != nil {
		return err
	}
	m.recordUpgradedSandbox(sandboxID, req.Name, labels, bindings)
	return nil
}

func microsandboxPortBindings(bindings []PortBinding) ([]microsandbox.PortBinding, error) {
	out := make([]microsandbox.PortBinding, 0, len(bindings))
	for _, binding := range bindings {
		hostPort, err := toUint16Port("host port", binding.HostPort)
		if err != nil {
			return nil, err
		}
		guestPort, err := toUint16Port("guest port", binding.GuestPort)
		if err != nil {
			return nil, err
		}
		out = append(out, microsandbox.PortBinding{
			Bind:      "0.0.0.0",
			HostPort:  hostPort,
			GuestPort: guestPort,
			Protocol:  microsandbox.PortProtocolTCP,
		})
	}
	return out, nil
}

func (m *MicrosandboxBackend) recordUpgradedSandbox(sandboxID, name string, labels map[string]string, bindings []PortBinding) {
	if name == "" {
		name = sandboxID
	}
	ports := make(map[int]int, len(bindings))
	for _, binding := range bindings {
		ports[binding.GuestPort] = binding.HostPort
	}
	m.mu.Lock()
	if m.ports == nil {
		m.ports = map[string]map[int]int{}
	}
	if m.sandboxes == nil {
		m.sandboxes = map[string]sandboxState{}
	}
	m.ports[sandboxID] = ports
	m.sandboxes[sandboxID] = sandboxState{
		ID:     sandboxID,
		Name:   name,
		Status: "running",
		Labels: cloneStringMap(labels),
		Ports:  cloneIntMap(ports),
	}
	m.mu.Unlock()
}
