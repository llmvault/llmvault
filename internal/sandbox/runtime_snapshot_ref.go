package sandbox

import (
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

const DefaultAgentSandboxSize = model.DefaultAgentSandboxSize

const (
	defaultRuntimeImageRepository   = "ghcr.io/usehivy/hivy-sandboxes-runtime"
	developerRuntimeImageRepository = "ghcr.io/usehivy/hivy-sandboxes-runtime-developers"
)

func AgentRuntimeTemplateRef(cfg *config.Config) string {
	return AgentRuntimeTemplateRefForSize(cfg, DefaultAgentSandboxSize)
}

func AgentRuntimeTemplateRefForSize(cfg *config.Config, size string) string {
	return AgentRuntimeTemplateRefForSandboxImageAndSize(cfg, model.SandboxImageDefault, size)
}

func AgentRuntimeTemplateRefForSandboxImageAndSize(cfg *config.Config, sandboxImage, size string) string {
	return RuntimeTemplateRefForImageRef(cfg, AgentRuntimeImageRef(cfg, sandboxImage), size)
}

func RuntimeTemplateRefForImageRef(cfg *config.Config, imageRef, size string) string {
	if cfg == nil {
		return ""
	}
	_ = size
	return strings.TrimSpace(imageRef)
}

func AgentRuntimeImageRef(cfg *config.Config, sandboxImage string) string {
	profile := model.NormalizeSandboxImage(sandboxImage)
	if cfg == nil {
		return runtimeImageRepository(profile) + ":latest"
	}
	if tag := strings.TrimSpace(cfg.SandboxesRuntimeImageTag); tag != "" {
		return runtimeImageRepository(profile) + ":" + tag
	}
	return runtimeImageRepository(profile) + ":latest"
}

func AgentRuntimeImageRefs(cfg *config.Config) []string {
	out := make([]string, 0, len(model.BuiltInSandboxImages()))
	seen := map[string]bool{}
	for _, profile := range model.BuiltInSandboxImages() {
		image := AgentRuntimeImageRef(cfg, profile)
		if image == "" || seen[image] {
			continue
		}
		seen[image] = true
		out = append(out, image)
	}
	return out
}

func runtimeImageRepository(sandboxImage string) string {
	switch model.NormalizeSandboxImage(sandboxImage) {
	case model.SandboxImageDeveloper:
		return developerRuntimeImageRepository
	default:
		return defaultRuntimeImageRepository
	}
}

func imageRepository(imageRef string) string {
	ref := strings.SplitN(strings.TrimSpace(imageRef), "@", 2)[0]
	if ref == "" {
		return ""
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		return ref[:lastColon]
	}
	return ref
}
