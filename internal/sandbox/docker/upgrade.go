package docker

import (
	"context"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"

	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) UpgradeSandbox(ctx context.Context, externalID string, opts sandbox.UpgradeSandboxOpts) (*sandbox.SandboxInfo, error) {
	if opts.TemplateRef == "" {
		return nil, fmt.Errorf("docker: UpgradeSandbox requires TemplateRef")
	}
	if err := d.ensureImage(ctx, opts.TemplateRef); err != nil {
		return nil, err
	}
	oldInfo, err := d.cli.ContainerInspect(ctx, externalID)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil, sandbox.ErrSandboxNotFound
		}
		return nil, fmt.Errorf("inspecting docker container %s: %w", externalID, err)
	}

	targetName := upgradeContainerName(externalID, oldInfo, opts.Name)
	workspaceVolume, err := d.prepareUpgradeWorkspace(ctx, externalID, oldInfo, targetName, opts.TemplateRef, opts.Labels)
	if err != nil {
		return nil, err
	}
	bindings := portBindingsFromInspect(oldInfo)
	if len(bindings) == 0 {
		bindings = defaultPortBindings()
	}
	rollbackImage := ""
	if oldInfo.Config != nil {
		rollbackImage = oldInfo.Config.Image
	}

	if oldInfo.State != nil && oldInfo.State.Running {
		timeout := 10
		if err := d.cli.ContainerStop(ctx, externalID, container.StopOptions{Timeout: &timeout}); err != nil {
			return nil, fmt.Errorf("stopping docker container before upgrade %s: %w", externalID, err)
		}
	}
	if err := d.cli.ContainerRemove(ctx, externalID, container.RemoveOptions{Force: true}); err != nil {
		return nil, fmt.Errorf("removing docker container before upgrade %s: %w", externalID, err)
	}

	err = d.createAndStartUpgradeContainer(ctx, targetName, opts.TemplateRef, opts, bindings, workspaceVolume)
	if err != nil {
		rollbackErr := d.rollbackUpgrade(ctx, targetName, rollbackImage, opts, bindings, workspaceVolume)
		if rollbackErr != nil {
			return nil, fmt.Errorf("upgrade create failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return nil, fmt.Errorf("upgrade create failed and rolled back: %w", err)
	}
	return &sandbox.SandboxInfo{ExternalID: targetName, Status: sandbox.StatusRunning}, nil
}

func (d *Driver) createAndStartUpgradeContainer(ctx context.Context, name, imageRef string, opts sandbox.UpgradeSandboxOpts, bindings nat.PortMap, workspaceVolume string) error {
	created, err := d.createDockerContainer(ctx, dockerContainerSpec{
		Name:            name,
		ImageRef:        imageRef,
		EnvVars:         opts.EnvVars,
		Labels:          opts.Labels,
		CPU:             opts.CPU,
		Memory:          opts.Memory,
		Disk:            opts.Disk,
		PortBindings:    bindings,
		WorkspaceVolume: workspaceVolume,
	})
	if err != nil {
		return fmt.Errorf("creating docker upgrade container: %w", err)
	}
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		_ = d.cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("starting docker upgrade container: %w", err)
	}
	return nil
}

func (d *Driver) rollbackUpgrade(ctx context.Context, name, imageRef string, opts sandbox.UpgradeSandboxOpts, bindings nat.PortMap, workspaceVolume string) error {
	if strings.TrimSpace(imageRef) == "" {
		return fmt.Errorf("previous docker image is unknown")
	}
	return d.createAndStartUpgradeContainer(ctx, name, imageRef, opts, bindings, workspaceVolume)
}

func upgradeContainerName(externalID string, info container.InspectResponse, fallback string) string {
	if name := strings.Trim(strings.TrimSpace(externalID), "/"); name != "" {
		return name
	}
	if info.Name != "" {
		return strings.Trim(info.Name, "/")
	}
	return containerName(fallback)
}

func portBindingsFromInspect(info container.InspectResponse) nat.PortMap {
	out := nat.PortMap{}
	if info.NetworkSettings == nil {
		return out
	}
	for port, bindings := range info.NetworkSettings.Ports {
		if len(bindings) == 0 {
			continue
		}
		copied := make([]nat.PortBinding, 0, len(bindings))
		for _, binding := range bindings {
			if strings.TrimSpace(binding.HostPort) == "" {
				continue
			}
			copied = append(copied, nat.PortBinding{
				HostIP:   binding.HostIP,
				HostPort: binding.HostPort,
			})
		}
		if len(copied) > 0 {
			out[port] = copied
		}
	}
	return out
}
