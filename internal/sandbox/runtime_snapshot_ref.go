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
	image := strings.TrimSpace(imageRef)
	if strings.TrimSpace(cfg.SandboxProviderID) != ProviderMicrosandbox {
		return image
	}
	alias := SnapshotAliasForImage(image, model.NormalizeTemplateSize(size))
	if alias == "" {
		return image
	}
	return alias
}

func AgentRuntimeImageRef(cfg *config.Config, sandboxImage string) string {
	profile := model.NormalizeSandboxImage(sandboxImage)
	if cfg == nil {
		return runtimeImageRepository(profile) + ":latest"
	}
	if tag := strings.TrimSpace(cfg.SandboxesRuntimeImageTag); tag != "" {
		return runtimeImageRepository(profile) + ":" + tag
	}

	fallback := strings.TrimSpace(cfg.SandboxesRuntimeBaseImage)
	if profile == model.SandboxImageDefault && fallback != "" {
		return fallback
	}
	if tag := imageTag(fallback); tag != "" {
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

func imageTag(imageRef string) string {
	ref := strings.SplitN(strings.TrimSpace(imageRef), "@", 2)[0]
	if ref == "" {
		return ""
	}
	if slash := strings.LastIndex(ref, "/"); slash >= 0 {
		ref = ref[slash+1:]
	}
	if colon := strings.LastIndex(ref, ":"); colon >= 0 && colon < len(ref)-1 {
		return ref[colon+1:]
	}
	return ""
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

func SnapshotAliasForImage(imageRef, size string) string {
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return ""
	}
	if size = strings.TrimSpace(size); size == "" {
		size = DefaultAgentSandboxSize
	}

	ref := strings.SplitN(imageRef, "@", 2)[0]
	if slash := strings.LastIndex(ref, "/"); slash >= 0 {
		ref = ref[slash+1:]
	}
	name := ref
	tag := "latest"
	if colon := strings.LastIndex(ref, ":"); colon >= 0 {
		name = ref[:colon]
		tag = ref[colon+1:]
	}

	name = snapshotAliasPart(name)
	tag = snapshotAliasPart(tag)
	size = snapshotAliasPart(size)
	if name == "" || tag == "" || size == "" {
		return ""
	}
	return name + "-" + tag + "-" + size
}

func snapshotAliasPart(raw string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
