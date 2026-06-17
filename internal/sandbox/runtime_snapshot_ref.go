package sandbox

import (
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

const DefaultAgentSandboxSize = model.DefaultAgentSandboxSize

func AgentRuntimeTemplateRef(cfg *config.Config) string {
	return AgentRuntimeTemplateRefForSize(cfg, DefaultAgentSandboxSize)
}

func AgentRuntimeTemplateRefForSize(cfg *config.Config, size string) string {
	if cfg == nil {
		return ""
	}
	image := strings.TrimSpace(cfg.SandboxesRuntimeBaseImage)
	if strings.TrimSpace(cfg.SandboxProviderID) != ProviderMicrosandbox {
		return image
	}
	alias := SnapshotAliasForImage(image, model.NormalizeTemplateSize(size))
	if alias == "" {
		return image
	}
	return alias
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
