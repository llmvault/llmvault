package bootstrap

import (
	"errors"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/sandbox/daytona"
	dockerprovider "github.com/usehivy/hivy/internal/sandbox/docker"
	microsandboxprovider "github.com/usehivy/hivy/internal/sandbox/microsandbox"
	railwayprovider "github.com/usehivy/hivy/internal/sandbox/railway"
)

var errSandboxProviderNotConfigured = errors.New("sandbox provider not configured")

func newSandboxProvider(cfg *config.Config) (sandbox.Provider, error) {
	providerID := strings.TrimSpace(cfg.SandboxProviderID)
	if providerID == "" {
		return nil, errSandboxProviderNotConfigured
	}

	switch providerID {
	case sandbox.ProviderDaytona:
		if strings.TrimSpace(cfg.DaytonaAPIKey) == "" {
			return nil, fmt.Errorf("%w: HIVY_DAYTONA_API_KEY is empty", errSandboxProviderNotConfigured)
		}
		return daytona.NewDriver(daytona.Config{
			APIURL: cfg.DaytonaAPIURL,
			APIKey: cfg.DaytonaAPIKey,
			Target: cfg.DaytonaTarget,
		})
	case sandbox.ProviderDocker:
		if strings.TrimSpace(cfg.SandboxDockerRuntimeOrigin) == "" {
			return nil, fmt.Errorf("%w: HIVY_SANDBOX_DOCKER_RUNTIME_ORIGIN is empty", errSandboxProviderNotConfigured)
		}
		return dockerprovider.NewDriver(dockerprovider.Config{
			Host:                 cfg.SandboxDockerHost,
			RuntimeOrigin:        cfg.SandboxDockerRuntimeOrigin,
			ContainerLabelPrefix: cfg.SandboxDockerContainerLabelPrefix,
		})
	case sandbox.ProviderRailway:
		if strings.TrimSpace(cfg.RailwayAPIToken) == "" {
			return nil, fmt.Errorf("%w: HIVY_RAILWAY_API_TOKEN is empty", errSandboxProviderNotConfigured)
		}
		return railwayprovider.NewDriver(railwayprovider.Config{
			APIToken:      cfg.RailwayAPIToken,
			ProjectID:     cfg.RailwayProjectID,
			EnvironmentID: cfg.RailwayEnvironmentID,
			Region:        cfg.RailwayRegion,
			RuntimePort:   cfg.RailwayRuntimePort,
		})
	case sandbox.ProviderMicrosandbox:
		if strings.TrimSpace(cfg.MicrosandboxControlURL) == "" {
			return nil, fmt.Errorf("%w: HIVY_MICROSANDBOX_CONTROL_URL is empty", errSandboxProviderNotConfigured)
		}
		if strings.TrimSpace(cfg.MicrosandboxControlAPIToken) == "" {
			return nil, fmt.Errorf("%w: HIVY_MICROSANDBOX_CONTROL_API_TOKEN is empty", errSandboxProviderNotConfigured)
		}
		return microsandboxprovider.NewDriver(microsandboxprovider.Config{
			ControlURL:   cfg.MicrosandboxControlURL,
			APIToken:     cfg.MicrosandboxControlAPIToken,
			RuntimePort:  sandbox.AgentSandboxPort,
			RuntimeImage: cfg.SandboxesRuntimeBaseImage,
		})
	default:
		return nil, fmt.Errorf("unsupported HIVY_SANDBOX_PROVIDER_ID %q", providerID)
	}
}
