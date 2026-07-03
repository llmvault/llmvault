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

// Systemd-as-PID-1 mode (Driver.systemd). The sandbox images ship systemd with
// hivy-runtime.service / hivy-appd.service enabled and an environment
// generator (/usr/lib/systemd/system-generators/hivy-sandbox-env-generator)
// that turns the container env into PassEnvironment= drop-ins for every unit,
// so overriding the entrypoint to systemd is all that is needed for the
// services to come up with the sandbox env. Verified empirically on a
// cgroup-v2 Docker host (Docker Desktop): with Privileged already true, systemd
// 257 mounts /run, /run/lock and its cgroup subtree itself — no tmpfs or
// /sys/fs/cgroup mounts are required.
const (
	// systemdInitPath is Debian's init symlink (→ /usr/lib/systemd/systemd).
	systemdInitPath = "/sbin/init"
	// systemdStopSignal asks systemd to start halt.target — the clean-shutdown
	// signal for a containerized systemd (systemd(1) "Signals").
	systemdStopSignal = "SIGRTMIN+3"
	// systemdStopTimeoutSeconds is the default `docker stop` grace period in
	// systemd mode. Shutdown completes in well under a second empirically;
	// the headroom covers sandboxes with busy user services.
	systemdStopTimeoutSeconds = 15
)

func (d *Driver) createDockerContainer(ctx context.Context, spec dockerContainerSpec) (container.CreateResponse, error) {
	cfg, hostCfg := d.containerConfigs(spec)

	created, err := d.cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, spec.Name)
	if err != nil && len(hostCfg.StorageOpt) > 0 && isUnsupportedStorageOptError(err) {
		hostCfg.StorageOpt = nil
		created, err = d.cli.ContainerCreate(ctx, cfg, hostCfg, &network.NetworkingConfig{}, nil, spec.Name)
	}
	return created, err
}

func (d *Driver) containerConfigs(spec dockerContainerSpec) (*container.Config, *container.HostConfig) {
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
	if d.systemd {
		// Leaving Cmd empty is deliberate: when the user config sets an
		// Entrypoint, the daemon does not inherit the image CMD, so systemd
		// starts with no stray arguments regardless of the image.
		cfg.Entrypoint = []string{systemdInitPath}
		cfg.StopSignal = systemdStopSignal
		stopTimeout := systemdStopTimeoutSeconds
		cfg.StopTimeout = &stopTimeout
		// Private cgroupns is the Docker default on cgroup-v2 hosts, but pin
		// it: systemd needs to own the root of its cgroup namespace.
		hostCfg.CgroupnsMode = container.CgroupnsMode("private")
	}
	return cfg, hostCfg
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
