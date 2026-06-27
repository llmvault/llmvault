package model

import "strings"

// TemplateSize defines the resource allocation for a sandbox template variant.
type TemplateSize struct {
	Name   string
	CPU    int
	Memory int // GB
	Disk   int // GB
}

const (
	DefaultAgentSandboxSize     = "small"
	DefaultHivyAgentSandboxSize = "nano"
)

// TemplateSizes maps size names to their resource allocations.
var TemplateSizes = map[string]TemplateSize{
	"nano":   {Name: "nano", CPU: 1, Memory: 1, Disk: 5},
	"small":  {Name: "small", CPU: 1, Memory: 2, Disk: 10},
	"medium": {Name: "medium", CPU: 2, Memory: 4, Disk: 20},
	"large":  {Name: "large", CPU: 4, Memory: 8, Disk: 40},
	"xlarge": {Name: "xlarge", CPU: 8, Memory: 16, Disk: 60},
}

// ValidTemplateSize returns true if the given size name is a valid template size.
func ValidTemplateSize(size string) bool {
	size = canonicalTemplateSize(size)
	_, ok := TemplateSizes[size]
	return ok
}

func NormalizeTemplateSize(size string) string {
	size = canonicalTemplateSize(size)
	if ValidTemplateSize(size) {
		return size
	}
	return DefaultAgentSandboxSize
}

func TemplateSizeSpec(size string) (TemplateSize, bool) {
	sz, ok := TemplateSizes[canonicalTemplateSize(size)]
	return sz, ok
}

func TemplateSizeForResources(cpu, memory, disk int) (string, bool) {
	for name, sz := range TemplateSizes {
		if sz.CPU == cpu && sz.Memory == memory && sz.Disk == disk {
			return name, true
		}
	}
	return "", false
}

func canonicalTemplateSize(size string) string {
	return strings.ToLower(strings.TrimSpace(size))
}
