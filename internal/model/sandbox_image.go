package model

import "strings"

const (
	SandboxImageDefault   = "default"
	SandboxImageDeveloper = "developer"
)

func NormalizeSandboxImage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return SandboxImageDefault
	}
	return value
}

func ValidSandboxImage(value string) bool {
	switch NormalizeSandboxImage(value) {
	case SandboxImageDefault, SandboxImageDeveloper:
		return true
	default:
		return false
	}
}

func BuiltInSandboxImages() []string {
	return []string{SandboxImageDefault, SandboxImageDeveloper}
}
