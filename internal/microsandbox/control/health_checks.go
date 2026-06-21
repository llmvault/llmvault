package control

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/usehivy/hivy/internal/microsandbox/model"
)

const (
	defaultHealthCheckTimeoutSeconds = 30
	defaultHealthCheckIntervalMS     = 250
)

type sandboxHealthCheckRequest struct {
	GuestPort      int    `json:"guest_port"`
	Type           string `json:"type"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	ExpectedStatus int    `json:"expected_status"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	IntervalMS     int    `json:"interval_ms"`
}

type runnerHealthCheckRequest struct {
	Type           string `json:"type"`
	Method         string `json:"method"`
	Path           string `json:"path"`
	ExpectedStatus int    `json:"expected_status"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	IntervalMS     int    `json:"interval_ms"`
}

func validateSandboxHealthChecks(checks []sandboxHealthCheckRequest, previewPorts []int) (map[int]sandboxHealthCheckRequest, error) {
	if len(checks) == 0 {
		return nil, nil
	}
	allowedPorts := make(map[int]struct{}, len(previewPorts))
	for _, port := range previewPorts {
		allowedPorts[port] = struct{}{}
	}
	out := make(map[int]sandboxHealthCheckRequest, len(checks))
	for _, check := range checks {
		if check.GuestPort <= 0 || check.GuestPort > 65535 {
			return nil, fmt.Errorf("health_checks.guest_port must be a valid port")
		}
		if _, ok := allowedPorts[check.GuestPort]; !ok {
			return nil, fmt.Errorf("health check guest_port %d is not previewable", check.GuestPort)
		}
		if _, exists := out[check.GuestPort]; exists {
			return nil, fmt.Errorf("duplicate health check for guest_port %d", check.GuestPort)
		}
		normalized, err := normalizeSandboxHealthCheck(check)
		if err != nil {
			return nil, err
		}
		out[check.GuestPort] = normalized
	}
	return out, nil
}

func normalizeSandboxHealthCheck(check sandboxHealthCheckRequest) (sandboxHealthCheckRequest, error) {
	check.Type = strings.ToLower(strings.TrimSpace(check.Type))
	if check.Type != "http" {
		return sandboxHealthCheckRequest{}, fmt.Errorf("health_checks.type must be http")
	}
	check.Method = strings.ToUpper(strings.TrimSpace(check.Method))
	if check.Method != "GET" && check.Method != "HEAD" {
		return sandboxHealthCheckRequest{}, fmt.Errorf("health_checks.method must be GET or HEAD")
	}
	check.Path = strings.TrimSpace(check.Path)
	if !validHealthCheckPath(check.Path) {
		return sandboxHealthCheckRequest{}, fmt.Errorf("health_checks.path must be a relative path beginning with /")
	}
	if check.ExpectedStatus < 100 || check.ExpectedStatus > 599 {
		return sandboxHealthCheckRequest{}, fmt.Errorf("health_checks.expected_status must be between 100 and 599")
	}
	if check.TimeoutSeconds < 0 {
		return sandboxHealthCheckRequest{}, fmt.Errorf("health_checks.timeout_seconds must be positive")
	}
	if check.TimeoutSeconds == 0 {
		check.TimeoutSeconds = defaultHealthCheckTimeoutSeconds
	}
	if check.IntervalMS < 0 {
		return sandboxHealthCheckRequest{}, fmt.Errorf("health_checks.interval_ms must be positive")
	}
	if check.IntervalMS == 0 {
		check.IntervalMS = defaultHealthCheckIntervalMS
	}
	return check, nil
}

func validHealthCheckPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return false
	}
	parsed, err := url.ParseRequestURI(path)
	return err == nil && !parsed.IsAbs() && parsed.Host == ""
}

func applyHealthCheckToPort(port *model.SandboxPort, check sandboxHealthCheckRequest) {
	if port == nil || check.Type == "" {
		return
	}
	port.HealthCheckType = check.Type
	port.HealthCheckMethod = check.Method
	port.HealthCheckPath = check.Path
	port.HealthCheckExpectedStatus = check.ExpectedStatus
	port.HealthCheckTimeoutSeconds = check.TimeoutSeconds
	port.HealthCheckIntervalMS = check.IntervalMS
}

func runnerHealthCheckForPort(port model.SandboxPort) *runnerHealthCheckRequest {
	if strings.TrimSpace(port.HealthCheckType) == "" {
		return nil
	}
	return &runnerHealthCheckRequest{
		Type:           port.HealthCheckType,
		Method:         port.HealthCheckMethod,
		Path:           port.HealthCheckPath,
		ExpectedStatus: port.HealthCheckExpectedStatus,
		TimeoutSeconds: port.HealthCheckTimeoutSeconds,
		IntervalMS:     port.HealthCheckIntervalMS,
	}
}
