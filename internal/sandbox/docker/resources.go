package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"

	"github.com/usehivy/hivy/internal/sandbox"
)

const bytesPerGiB = int64(1024 * 1024 * 1024)

// Default container limits when CreateSandboxOpts carries no size; without them a
// runaway agent can take down every co-located sandbox (PidsLimit caps fork bombs).
const (
	defaultCPUCores  = 2
	defaultMemoryGB  = 4
	defaultPidsLimit = int64(1024)
)

func resourceLimits(cpu, memoryGB int) container.Resources {
	if cpu <= 0 {
		cpu = defaultCPUCores
	}
	if memoryGB <= 0 {
		memoryGB = defaultMemoryGB
	}
	pids := defaultPidsLimit
	return container.Resources{
		NanoCPUs:  int64(cpu) * 1_000_000_000,
		Memory:    int64(memoryGB) * bytesPerGiB,
		PidsLimit: &pids,
	}
}

func (d *Driver) GetResourceUsage(ctx context.Context, externalID string) (*sandbox.ResourceUsage, error) {
	stats, err := d.readStats(ctx, externalID)
	if err != nil {
		return nil, err
	}
	inspect, err := d.cli.ContainerInspect(ctx, externalID)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil, sandbox.ErrSandboxNotFound
		}
		return nil, fmt.Errorf("inspecting docker container %s: %w", externalID, err)
	}

	return &sandbox.ResourceUsage{
		MemoryLimitBytes:  uint64ToInt64(stats.MemoryStats.Limit),
		MemoryUsedBytes:   uint64ToInt64(stats.MemoryStats.Usage),
		MemoryPeakBytes:   uint64ToInt64(stats.MemoryStats.MaxUsage),
		CPUQuota:          cpuQuota(inspect.HostConfig),
		CPUUsageUsec:      uint64ToInt64(stats.CPUStats.CPUUsage.TotalUsage / 1000),
		CPUThrottledCount: uint64ToInt64(stats.CPUStats.ThrottlingData.ThrottledPeriods),
		PIDCount:          uint64ToInt64(stats.PidsStats.Current),
	}, nil
}

func uint64ToInt64(value uint64) int64 {
	const maxInt64 = int64(1<<63 - 1)
	if value > uint64(maxInt64) {
		return maxInt64
	}
	return int64(value)
}

func (d *Driver) readStats(ctx context.Context, externalID string) (*container.StatsResponse, error) {
	body, err := d.cli.ContainerStats(ctx, externalID, false)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil, sandbox.ErrSandboxNotFound
		}
		return nil, fmt.Errorf("reading docker stats %s: %w", externalID, err)
	}
	defer body.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(body.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decoding docker stats %s: %w", externalID, err)
	}
	return &stats, nil
}

func cpuQuota(hostConfig *container.HostConfig) string {
	if hostConfig == nil {
		return ""
	}
	if hostConfig.CPUQuota > 0 || hostConfig.CPUPeriod > 0 {
		return fmt.Sprintf("%d %d", hostConfig.CPUQuota, hostConfig.CPUPeriod)
	}
	if hostConfig.NanoCPUs > 0 {
		return strconv.FormatInt(hostConfig.NanoCPUs, 10)
	}
	return ""
}
