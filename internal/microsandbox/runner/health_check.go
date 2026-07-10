package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
)

const (
	defaultHealthCheckTimeout  = 30 * time.Second
	defaultHealthCheckInterval = 250 * time.Millisecond
	healthCheckProbeTimeout    = 2 * time.Second
)

func waitForHTTPHealthCheck(ctx context.Context, sandboxID string, guestPort, hostPort int, check HealthCheckConfig) error {
	normalized, err := normalizeHealthCheckConfig(check)
	if err != nil {
		return err
	}
	started := time.Now()
	target := "http://" + netJoinLocalhost(hostPort) + normalized.Path
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	deadline := time.Now().Add(normalized.timeout)
	last := "not checked"
	for {
		if err := ctx.Err(); err != nil {
			err = healthCheckTimeoutError(normalized, err.Error(), last)
			logHealthCheckFailure(ctx, sandboxID, guestPort, hostPort, normalized.Path, started, err)
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			err := healthCheckTimeoutError(normalized, normalized.timeout.String(), last)
			logHealthCheckFailure(ctx, sandboxID, guestPort, hostPort, normalized.Path, started, err)
			return err
		}
		probeCtx, cancel := context.WithTimeout(ctx, minDuration(healthCheckProbeTimeout, remaining))
		req, reqErr := http.NewRequestWithContext(probeCtx, normalized.Method, target, nil)
		if reqErr != nil {
			cancel()
			return reqErr
		}
		resp, doErr := client.Do(req)
		if doErr != nil {
			// Only treat this as a "transient" error worth retrying when
			// the probe didn't hit its deadline; a context-deadline error
			// means we've already spent the whole timeout on one probe and
			// should fall through to the timeout path below instead of
			// clobbering the last observed status with a deadline string.
			if probeCtx.Err() == nil {
				last = doErr.Error()
			} else {
				last = fmt.Sprintf("status=%d (probe deadline)", lastStatusOrZero(resp))
			}
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			if resp.StatusCode == normalized.ExpectedStatus {
				logging.FromContext(ctx).InfoContext(ctx, "sandbox health check ready",
					"sandbox_id", sandboxID,
					"guest_port", guestPort,
					"host_port", hostPort,
					"path", normalized.Path,
					"status", resp.StatusCode,
					"duration_ms", time.Since(started).Milliseconds(),
				)
				cancel()
				return nil
			}
			last = fmt.Sprintf("status=%d", resp.StatusCode)
		}
		cancel()
		sleepFor := minDuration(normalized.interval, time.Until(deadline))
		if sleepFor <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			err := healthCheckTimeoutError(normalized, ctx.Err().Error(), last)
			logHealthCheckFailure(ctx, sandboxID, guestPort, hostPort, normalized.Path, started, err)
			return err
		case <-time.After(sleepFor):
		}
	}
}

func healthCheckTimeoutError(check normalizedHealthCheck, limit, last string) error {
	return fmt.Errorf("health check %s %s did not return %d within %s: %s",
		check.Method, check.Path, check.ExpectedStatus, limit, last)
}

func logHealthCheckFailure(ctx context.Context, sandboxID string, guestPort, hostPort int, path string, started time.Time, err error) {
	logging.FromContext(ctx).WarnContext(ctx, "sandbox health check failed",
		"sandbox_id", sandboxID,
		"guest_port", guestPort,
		"host_port", hostPort,
		"path", path,
		"duration_ms", time.Since(started).Milliseconds(),
		"error", err.Error(),
	)
}

type normalizedHealthCheck struct {
	Type           string
	Method         string
	Path           string
	ExpectedStatus int
	timeout        time.Duration
	interval       time.Duration
}

func normalizeHealthCheckConfig(check HealthCheckConfig) (normalizedHealthCheck, error) {
	out := normalizedHealthCheck{
		Type:           strings.ToLower(strings.TrimSpace(check.Type)),
		Method:         strings.ToUpper(strings.TrimSpace(check.Method)),
		Path:           strings.TrimSpace(check.Path),
		ExpectedStatus: check.ExpectedStatus,
	}
	if out.Type != "http" {
		return normalizedHealthCheck{}, fmt.Errorf("health_check.type must be http")
	}
	if out.Method != http.MethodGet && out.Method != http.MethodHead {
		return normalizedHealthCheck{}, fmt.Errorf("health_check.method must be GET or HEAD")
	}
	if !validRunnerHealthCheckPath(out.Path) {
		return normalizedHealthCheck{}, fmt.Errorf("health_check.path must be a relative path beginning with /")
	}
	if out.ExpectedStatus < 100 || out.ExpectedStatus > 599 {
		return normalizedHealthCheck{}, fmt.Errorf("health_check.expected_status must be between 100 and 599")
	}
	if check.TimeoutSeconds < 0 {
		return normalizedHealthCheck{}, fmt.Errorf("health_check.timeout_seconds must be positive")
	}
	out.timeout = time.Duration(check.TimeoutSeconds) * time.Second
	if out.timeout == 0 {
		out.timeout = defaultHealthCheckTimeout
	}
	if check.IntervalMS < 0 {
		return normalizedHealthCheck{}, fmt.Errorf("health_check.interval_ms must be positive")
	}
	out.interval = time.Duration(check.IntervalMS) * time.Millisecond
	if out.interval == 0 {
		out.interval = defaultHealthCheckInterval
	}
	return out, nil
}

func validRunnerHealthCheckPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return false
	}
	parsed, err := url.ParseRequestURI(path)
	return err == nil && !parsed.IsAbs() && parsed.Host == ""
}

func netJoinLocalhost(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func lastStatusOrZero(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}
