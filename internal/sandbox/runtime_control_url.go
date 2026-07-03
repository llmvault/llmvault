package sandbox

import (
	"net"
	"net/url"
	"strings"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) runtimeControlURL(raw string) string {
	return runtimeControlURL(o.cfg, o.providerID(), raw)
}

func (o *Orchestrator) sandboxForRuntimeControl(sb *model.Sandbox) *model.Sandbox {
	if sb == nil {
		return nil
	}
	controlURL := o.runtimeControlURL(sb.RuntimeURL)
	if controlURL == sb.RuntimeURL {
		return sb
	}
	copy := *sb
	copy.RuntimeURL = controlURL
	return &copy
}

// RuntimeControlURL exposes the docker control-origin rewrite to non-agent
// orchestration (the apps deploy service): when the Go server runs inside
// compose, sandbox endpoints returned as http://127.0.0.1:<port> must be
// reached via HIVY_SANDBOX_DOCKER_CONTROL_ORIGIN (host.docker.internal).
// Non-docker providers and empty config pass through unchanged.
func RuntimeControlURL(cfg *config.Config, providerID, raw string) string {
	return runtimeControlURL(cfg, providerID, raw)
}

func runtimeControlURL(cfg *config.Config, providerID, raw string) string {
	runtimeURL := strings.TrimSpace(raw)
	if cfg == nil || providerID != ProviderDocker {
		return runtimeURL
	}
	controlOrigin := strings.TrimRight(strings.TrimSpace(cfg.SandboxDockerControlOrigin), "/")
	if controlOrigin == "" {
		return runtimeURL
	}
	rewritten, ok := replaceRuntimeOrigin(runtimeURL, controlOrigin)
	if !ok {
		return runtimeURL
	}
	return rewritten
}

func replaceRuntimeOrigin(raw, controlOrigin string) (string, bool) {
	runtimeURL, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || runtimeURL.Scheme == "" || runtimeURL.Host == "" {
		return raw, false
	}
	controlURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(controlOrigin), "/"))
	if err != nil || controlURL.Scheme == "" || controlURL.Host == "" {
		return raw, false
	}
	if controlURL.Path != "" || controlURL.RawQuery != "" || controlURL.Fragment != "" || controlURL.Port() != "" {
		return raw, false
	}
	runtimeURL.Scheme = controlURL.Scheme
	if port := runtimeURL.Port(); port != "" {
		runtimeURL.Host = net.JoinHostPort(controlURL.Hostname(), port)
	} else {
		runtimeURL.Host = controlURL.Host
	}
	return runtimeURL.String(), true
}
