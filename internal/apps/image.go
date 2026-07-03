package apps

import (
	"strings"

	"github.com/usehivy/hivy/internal/config"
)

// appImageRepository hosts the app sandbox image built from
// sandboxes/app/Dockerfile (tini → entrypoint → hivy-appd).
const appImageRepository = "ghcr.io/usehivy/hivy-app"

// Container ports inside the app sandbox: appd control on 7080 (same
// convention as the agent runtime, published by the docker provider's
// default bindings) and the app itself on 8080.
const (
	appdPort = 7080
	appPort  = 8080
)

// AppImageRef resolves the app sandbox image, pinned by
// HIVY_SANDBOXES_APP_IMAGE_TAG (mirrors AgentRuntimeImageRef /
// HIVY_SANDBOXES_RUNTIME_IMAGE_TAG).
func AppImageRef(cfg *config.Config) string {
	if cfg != nil {
		if tag := strings.TrimSpace(cfg.SandboxesAppImageTag); tag != "" {
			return appImageRepository + ":" + tag
		}
	}
	return appImageRepository + ":latest"
}
