package daytona

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	defaultDaytonaMaxCPU              = 4
	defaultDaytonaMaxMemoryGB         = 8
	defaultDaytonaMaxDiskGB           = 10
	defaultRuntimeRepository          = "ghcr.io/usehivy/hivy-sandboxes-runtime"
	developerRuntimeRepository        = "ghcr.io/usehivy/hivy-sandboxes-runtime-developers"
	defaultDaytonaRuntimeRepository   = "ghcr.io/usehivy/hivy-sandboxes-runtime-daytona"
	developerDaytonaRuntimeRepository = "ghcr.io/usehivy/hivy-sandboxes-runtime-developers-daytona"
	defaultSnapshotPrefix             = "hivy-sandboxes-runtime-daytona"
	developerSnapshotPrefix           = "hivy-sandboxes-runtime-developers-daytona"
)

type resourceLimits struct {
	CPU    int
	Memory int
	Disk   int
}

var defaultDaytonaResourceLimits = resourceLimits{
	CPU:    defaultDaytonaMaxCPU,
	Memory: defaultDaytonaMaxMemoryGB,
	Disk:   defaultDaytonaMaxDiskGB,
}

func normalizedResourceLimits(cpu, memory, disk int) resourceLimits {
	limits := defaultDaytonaResourceLimits
	if cpu > 0 {
		limits.CPU = cpu
	}
	if memory > 0 {
		limits.Memory = memory
	}
	if disk > 0 {
		limits.Disk = disk
	}
	return limits
}

type runtimeResourceAdjustment struct {
	Size            string
	RequestedCPU    int
	RequestedMemory int
	RequestedDisk   int
	CPU             int
	Memory          int
	Disk            int
}

func (a runtimeResourceAdjustment) Changed() bool {
	return a.RequestedCPU != a.CPU || a.RequestedMemory != a.Memory || a.RequestedDisk != a.Disk
}

func daytonaRuntimeResourceAdjustment(opts sandbox.CreateSandboxOpts, limits resourceLimits) (runtimeResourceAdjustment, bool) {
	if !isHivyRuntimeImageRef(opts.TemplateRef) {
		return runtimeResourceAdjustment{}, false
	}
	size, ok := model.TemplateSizeForResources(opts.CPU, opts.Memory, opts.Disk)
	if !ok {
		return runtimeResourceAdjustment{}, false
	}
	cpu := min(opts.CPU, limits.CPU)
	memory := min(opts.Memory, limits.Memory)
	if memory < 1 {
		memory = 1
	}
	disk := opts.Disk
	if isDeveloperRuntimeImageRef(opts.TemplateRef) && disk < limits.Disk {
		disk = limits.Disk
	}
	if disk > limits.Disk {
		disk = limits.Disk
	}
	return runtimeResourceAdjustment{
		Size:            size,
		RequestedCPU:    opts.CPU,
		RequestedMemory: opts.Memory,
		RequestedDisk:   opts.Disk,
		CPU:             cpu,
		Memory:          memory,
		Disk:            disk,
	}, true
}

func resolveRuntimeSnapshotRef(opts sandbox.CreateSandboxOpts) (string, error) {
	repository, tag, ok := splitTaggedImageRef(opts.TemplateRef)
	if !ok {
		return strings.TrimSpace(opts.TemplateRef), nil
	}

	prefix := ""
	switch repository {
	case defaultRuntimeRepository, defaultDaytonaRuntimeRepository:
		prefix = defaultSnapshotPrefix
	case developerRuntimeRepository, developerDaytonaRuntimeRepository:
		prefix = developerSnapshotPrefix
	default:
		return strings.TrimSpace(opts.TemplateRef), nil
	}

	size, ok := model.TemplateSizeForResources(opts.CPU, opts.Memory, opts.Disk)
	if !ok {
		return "", fmt.Errorf("unsupported Hivy resource tuple cpu=%d memory=%d disk=%d", opts.CPU, opts.Memory, opts.Disk)
	}
	version := strings.TrimSuffix(strings.TrimPrefix(tag, "v"), "-amd64")
	version = dashedSnapshotVersion(version)
	if version == "" || version == "latest" || version == "stable" || version == "lts" {
		return "", fmt.Errorf("image tag %q is not an immutable release version", tag)
	}
	return fmt.Sprintf("%s-%s-%s-v1", prefix, version, size), nil
}

func isHivyRuntimeImageRef(imageRef string) bool {
	repository, _, ok := splitTaggedImageRef(imageRef)
	if !ok {
		return false
	}
	switch repository {
	case defaultRuntimeRepository, developerRuntimeRepository, defaultDaytonaRuntimeRepository, developerDaytonaRuntimeRepository:
		return true
	default:
		return false
	}
}

func isDeveloperRuntimeImageRef(imageRef string) bool {
	repository, _, ok := splitTaggedImageRef(imageRef)
	if !ok {
		return false
	}
	return repository == developerRuntimeRepository || repository == developerDaytonaRuntimeRepository
}

func splitTaggedImageRef(imageRef string) (repository, tag string, ok bool) {
	trimmed := strings.TrimSpace(imageRef)
	if trimmed == "" || strings.Contains(trimmed, "@") {
		return "", "", false
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon <= lastSlash || lastColon == len(trimmed)-1 {
		return "", "", false
	}
	return trimmed[:lastColon], trimmed[lastColon+1:], true
}

func dashedSnapshotVersion(version string) string {
	var out strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(version)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}
