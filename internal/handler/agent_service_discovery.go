package handler

import (
	"strings"
)

func serviceDiscoveryProviderSupported(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "glitchtip", "linear", "notion", "railway", "slack", "vercel":
		return true
	default:
		return false
	}
}
