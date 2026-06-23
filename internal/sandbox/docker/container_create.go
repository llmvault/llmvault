package docker

import (
	"context"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"github.com/usehivy/hivy/internal/sandbox"
)

type dockerContainerSpec struct {
	Name                string
	ImageRef            string
	EnvVars             map[string]string
	Labels              map[string]string
	CPU                 int
	Memory              int
	Disk                int
	PortBindings        nat.PortMap
	WorkspaceVolume     string
	PublishDefaultPorts bool
}

func (d *Driver) createDockerContainer(ctx context.Context, spec dockerContainerSpec) (container.CreateResponse, error) {
	bindings := spec.PortBindings
	if len(bindings) == 0 && spec.PublishDefaultPorts {
		bindings = defaultPortBindings()
	}
	hostCfg := &container.HostConfig{
		NetworkMode:  "bridge",
		PortBindings: bindings,
		Privileged:   true,
		Resources:    resourceLimits(spec.CPU, spec.Memory),
		StorageOpt:   storageOpt(spec.Disk),
		ExtraHosts:   []string{"host.docker.internal:host-gateway"},
	}
	if spec.WorkspaceVolume != "" {
		hostCfg.Mounts = []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: spec.WorkspaceVolume,
			Target: workspaceMountPath,
		}}
	}
	cfg := &container.Config{
		Image:        spec.ImageRef,
		Env:          envList(spec.EnvVars),
		Labels:       d.labels(spec.Labels),
		ExposedPorts: portSetFromBindings(bindings),
	}

	created, err := d.cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, spec.Name)
	if err != nil && len(hostCfg.StorageOpt) > 0 && isUnsupportedStorageOptError(err) {
		hostCfg.StorageOpt = nil
		created, err = d.cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, spec.Name)
	}
	return created, err
}

func defaultPortBindings() nat.PortMap {
	bindings := nat.PortMap{}
	for _, port := range []int{sandbox.RuntimePort, sandbox.AgentSandboxPort, 8080} {
		key := nat.Port(strconv.Itoa(port) + "/tcp")
		bindings[key] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}}
	}
	return bindings
}

func portSetFromBindings(bindings nat.PortMap) nat.PortSet {
	ports := nat.PortSet{}
	for port := range bindings {
		ports[port] = struct{}{}
	}
	return ports
}
