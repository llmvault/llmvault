package docker

import (
	"slices"
	"testing"

	"github.com/docker/docker/api/types/container"

	"github.com/usehivy/hivy/internal/model"
)

func TestContainerConfigsTiniModeUnchanged(t *testing.T) {
	t.Parallel()

	driver := &Driver{labelPrefix: "hivy"}
	cfg, hostCfg := driver.containerConfigs(dockerContainerSpec{
		ImageRef:            "example:latest",
		Labels:              map[string]string{"sandbox_image": model.SandboxImageDeveloper},
		PublishDefaultPorts: true,
	})

	if len(cfg.Entrypoint) != 0 {
		t.Fatalf("tini mode Entrypoint = %v, want image default", cfg.Entrypoint)
	}
	if cfg.StopSignal != "" {
		t.Fatalf("tini mode StopSignal = %q, want image default", cfg.StopSignal)
	}
	if cfg.StopTimeout != nil {
		t.Fatalf("tini mode StopTimeout = %v, want nil", *cfg.StopTimeout)
	}
	if hostCfg.CgroupnsMode != "" {
		t.Fatalf("tini mode CgroupnsMode = %q, want daemon default", hostCfg.CgroupnsMode)
	}
	if !hostCfg.Privileged {
		t.Fatal("Privileged should be true")
	}
	if !slices.Contains(cfg.Env, runtimeStartDockerEnv+"=1") {
		t.Fatalf("container env = %v, want nested Docker startup enabled", cfg.Env)
	}
}

func TestContainerConfigsSystemdMode(t *testing.T) {
	t.Parallel()

	driver := &Driver{labelPrefix: "hivy", systemd: true}
	cfg, hostCfg := driver.containerConfigs(dockerContainerSpec{
		ImageRef:            "example:latest",
		Labels:              map[string]string{"sandbox_image": model.SandboxImageDeveloper},
		PublishDefaultPorts: true,
	})

	if len(cfg.Entrypoint) != 1 || cfg.Entrypoint[0] != systemdInitPath {
		t.Fatalf("systemd mode Entrypoint = %v, want [%s]", cfg.Entrypoint, systemdInitPath)
	}
	if len(cfg.Cmd) != 0 {
		t.Fatalf("systemd mode Cmd = %v, want empty", cfg.Cmd)
	}
	if cfg.StopSignal != systemdStopSignal {
		t.Fatalf("systemd mode StopSignal = %q, want %q", cfg.StopSignal, systemdStopSignal)
	}
	if cfg.StopTimeout == nil || *cfg.StopTimeout != systemdStopTimeoutSeconds {
		t.Fatalf("systemd mode StopTimeout = %v, want %d", cfg.StopTimeout, systemdStopTimeoutSeconds)
	}
	if hostCfg.CgroupnsMode != container.CgroupnsMode("private") {
		t.Fatalf("systemd mode CgroupnsMode = %q, want private", hostCfg.CgroupnsMode)
	}
	if !hostCfg.Privileged {
		t.Fatal("Privileged should be true")
	}
}
