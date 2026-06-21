package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"github.com/usehivy/hivy/internal/sandbox"
)

func (d *Driver) CreateSandbox(ctx context.Context, opts sandbox.CreateSandboxOpts) (*sandbox.SandboxInfo, error) {
	if opts.TemplateRef == "" {
		return nil, fmt.Errorf("docker: CreateSandbox requires TemplateRef")
	}
	if err := d.ensureImage(ctx, opts.TemplateRef); err != nil {
		return nil, err
	}

	exposed, bindings := exposedPorts()
	hostCfg := &container.HostConfig{
		NetworkMode:  "bridge",
		PortBindings: bindings,
		Privileged:   true,
		Resources:    resourceLimits(opts.CPU, opts.Memory),
		StorageOpt:   storageOpt(opts.Disk),
		ExtraHosts:   []string{"host.docker.internal:host-gateway"},
	}
	cfg := &container.Config{
		Image:        opts.TemplateRef,
		Env:          envList(opts.EnvVars),
		Labels:       d.labels(opts.Labels),
		ExposedPorts: exposed,
	}

	created, err := d.cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, containerName(opts.Name))
	if err != nil && len(hostCfg.StorageOpt) > 0 && isUnsupportedStorageOptError(err) {
		hostCfg.StorageOpt = nil
		created, err = d.cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, containerName(opts.Name))
	}
	if err != nil {
		return nil, fmt.Errorf("creating docker container: %w", err)
	}
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		_ = d.cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		return nil, fmt.Errorf("starting docker container: %w", err)
	}
	return &sandbox.SandboxInfo{ExternalID: created.ID, Status: sandbox.StatusRunning}, nil
}

func isUnsupportedStorageOptError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "--storage-opt is supported only") ||
		strings.Contains(msg, "storage options are not supported") ||
		strings.Contains(msg, "storage opts are not supported") ||
		strings.Contains(msg, "storageopt is not supported")
}

func (d *Driver) StartSandbox(ctx context.Context, externalID string) error {
	if err := d.cli.ContainerStart(ctx, externalID, container.StartOptions{}); err != nil {
		if cerrdefs.IsNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("starting docker container %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) StopSandbox(ctx context.Context, externalID string) error {
	timeout := 10
	if err := d.cli.ContainerStop(ctx, externalID, container.StopOptions{Timeout: &timeout}); err != nil {
		if cerrdefs.IsNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("stopping docker container %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) ArchiveSandbox(ctx context.Context, externalID string) error {
	return d.StopSandbox(ctx, externalID)
}

func (d *Driver) DeleteSandbox(ctx context.Context, externalID string) error {
	err := d.cli.ContainerRemove(ctx, externalID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	if cerrdefs.IsNotFound(err) {
		return sandbox.ErrSandboxNotFound
	}
	return err
}

func (d *Driver) GetStatus(ctx context.Context, externalID string) (sandbox.SandboxStatus, error) {
	info, err := d.cli.ContainerInspect(ctx, externalID)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return sandbox.StatusError, sandbox.ErrSandboxNotFound
		}
		return sandbox.StatusError, fmt.Errorf("inspecting docker container %s: %w", externalID, err)
	}
	if info.State == nil {
		return sandbox.StatusError, nil
	}
	switch {
	case info.State.Running:
		return sandbox.StatusRunning, nil
	case info.State.Status == "created", info.State.Status == "exited", info.State.Status == "paused":
		return sandbox.StatusStopped, nil
	default:
		return sandbox.StatusError, nil
	}
}

func (d *Driver) ensureImage(ctx context.Context, ref string) error {
	if _, err := d.cli.ImageInspect(ctx, ref); err == nil {
		return nil
	} else if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting docker image %s: %w", ref, err)
	}
	body, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling docker image %s: %w", ref, err)
	}
	defer body.Close()
	if _, err := io.Copy(io.Discard, body); err != nil {
		return fmt.Errorf("reading docker image pull stream %s: %w", ref, err)
	}
	return nil
}

func exposedPorts() (nat.PortSet, nat.PortMap) {
	ports := nat.PortSet{}
	bindings := nat.PortMap{}
	for _, port := range []int{sandbox.RuntimePort, sandbox.AgentSandboxPort, 8080} {
		key := nat.Port(strconv.Itoa(port) + "/tcp")
		ports[key] = struct{}{}
		bindings[key] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}}
	}
	return ports, bindings
}
