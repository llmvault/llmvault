package sandbox

import (
	"fmt"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

type WarmPoolProfile struct {
	Mode         string `json:"mode"`
	ImageKind    string `json:"image_kind"`
	RuntimeImage string `json:"runtime_image"`
	SandboxSize  string `json:"sandbox_size"`
	CPU          int    `json:"cpu"`
	Memory       int    `json:"memory"`
	Disk         int    `json:"disk"`
}

func AgentWarmPoolProfile(cfg *config.Config, sandboxImage, sandboxSize string) WarmPoolProfile {
	imageKind := model.NormalizeSandboxImage(sandboxImage)
	size := model.NormalizeTemplateSize(sandboxSize)
	spec, _ := model.TemplateSizeSpec(size)
	return WarmPoolProfile{
		Mode:         model.SandboxWarmSlotModeAgent,
		ImageKind:    imageKind,
		RuntimeImage: AgentRuntimeImageRef(cfg, imageKind),
		SandboxSize:  size,
		CPU:          spec.CPU,
		Memory:       spec.Memory,
		Disk:         spec.Disk,
	}
}

func ConfiguredWarmPoolProfiles(cfg *config.Config) []WarmPoolProfile {
	profiles := []WarmPoolProfile{}
	for _, imageKind := range model.BuiltInSandboxImages() {
		profile := AgentWarmPoolProfile(cfg, imageKind, model.DefaultAgentSandboxSize)
		if warmPoolDesiredCount(cfg, profile) <= 0 {
			continue
		}
		for _, size := range warmPoolSandboxSizes() {
			profiles = append(profiles, AgentWarmPoolProfile(cfg, imageKind, size))
		}
	}
	return profiles
}

func normalizeWarmPoolProfile(cfg *config.Config, profile WarmPoolProfile) WarmPoolProfile {
	mode := strings.TrimSpace(profile.Mode)
	if mode == "" {
		mode = model.SandboxWarmSlotModeAgent
	}
	imageKind := model.NormalizeSandboxImage(profile.ImageKind)
	size := model.NormalizeTemplateSize(profile.SandboxSize)
	spec, _ := model.TemplateSizeSpec(size)
	runtimeImage := strings.TrimSpace(profile.RuntimeImage)
	if runtimeImage == "" {
		runtimeImage = AgentRuntimeImageRef(cfg, imageKind)
	}
	cpu := profile.CPU
	if cpu <= 0 {
		cpu = spec.CPU
	}
	memory := profile.Memory
	if memory <= 0 {
		memory = spec.Memory
	}
	disk := profile.Disk
	if disk <= 0 {
		disk = spec.Disk
	}
	return WarmPoolProfile{
		Mode:         mode,
		ImageKind:    imageKind,
		RuntimeImage: runtimeImage,
		SandboxSize:  size,
		CPU:          cpu,
		Memory:       memory,
		Disk:         disk,
	}
}

func warmPoolDesiredCount(cfg *config.Config, profile WarmPoolProfile) int {
	if cfg == nil {
		return 0
	}
	var desired int
	switch model.NormalizeSandboxImage(profile.ImageKind) {
	case model.SandboxImageDeveloper:
		desired = cfg.SandboxWarmPoolDeveloperSize
	default:
		desired = cfg.SandboxWarmPoolDefaultSize
	}
	if desired < 0 {
		return 0
	}
	return desired
}

func (p WarmPoolProfile) logValue() string {
	return fmt.Sprintf("%s/%s/%s/%d/%d/%d/%s", p.Mode, p.ImageKind, p.SandboxSize, p.CPU, p.Memory, p.Disk, p.RuntimeImage)
}

func warmPoolSandboxSizes() []string {
	sizes := make([]string, 0, len(model.TemplateSizes))
	for size := range model.TemplateSizes {
		sizes = append(sizes, size)
	}
	sort.Strings(sizes)
	return sizes
}
