package docker

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/docker/docker/client"

	"github.com/usehivy/hivy/internal/sandbox"
)

type Config struct {
	Host                 string
	RuntimeOrigin        string
	ContainerLabelPrefix string
}

type Driver struct {
	cli           *client.Client
	runtimeOrigin string
	labelPrefix   string
}

func NewDriver(cfg Config) (*Driver, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if strings.TrimSpace(cfg.Host) != "" {
		opts = append(opts, client.WithHost(strings.TrimSpace(cfg.Host)))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	labelPrefix := strings.TrimSpace(cfg.ContainerLabelPrefix)
	if labelPrefix == "" {
		labelPrefix = "hivy"
	}
	runtimeOrigin, err := normalizeRuntimeOrigin(cfg.RuntimeOrigin)
	if err != nil {
		return nil, err
	}
	return &Driver{
		cli:           cli,
		runtimeOrigin: runtimeOrigin,
		labelPrefix:   labelPrefix,
	}, nil
}

func (d *Driver) ID() string { return sandbox.ProviderDocker }

func (d *Driver) Validate(ctx context.Context) error {
	if d.runtimeOrigin == "" {
		return fmt.Errorf("HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN is required for docker sandbox provider")
	}
	if _, err := d.cli.Ping(ctx); err != nil {
		return fmt.Errorf("ping docker daemon: %w", err)
	}
	return nil
}

func (d *Driver) RuntimeLayout() sandbox.RuntimeLayout {
	return sandbox.RuntimeLayout{
		AgentRepoDir:     "/work/repos",
		WorkspaceRepoDir: "/workspace/repos",
	}
}

func (d *Driver) labels(input map[string]string) map[string]string {
	labels := map[string]string{
		d.labelPrefix + ".provider": "docker",
	}
	for key, value := range input {
		labels[d.labelPrefix+"."+key] = value
	}
	return labels
}

func normalizeRuntimeOrigin(raw string) (string, error) {
	origin := strings.TrimRight(strings.TrimSpace(raw), "/")
	if origin == "" {
		return "", nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN must be an absolute origin")
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN must not include path, query, or fragment")
	}
	if parsed.Port() != "" {
		return "", fmt.Errorf("HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN must not include a port; Docker published-port mode appends the sandbox port")
	}
	if strings.EqualFold(parsed.Hostname(), "host.docker.internal") {
		return "", fmt.Errorf("HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN must be reachable by both browsers and containers, not host.docker.internal")
	}
	return origin, nil
}
