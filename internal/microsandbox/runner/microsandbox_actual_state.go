package runner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	microsandbox "github.com/superradcompany/microsandbox/sdk/go"
)

const (
	agentRuntimeGuestPort = 7080
	actualProbeTimeout    = 750 * time.Millisecond
	stopVerifyTimeout     = 30 * time.Second
	startVerifyTimeout    = 90 * time.Second
	lifecyclePollInterval = 250 * time.Millisecond
)

type actualSandboxState struct {
	NativeStatus         string
	ProcessPIDs          []int
	DockerDiskFDs        int
	OpenPorts            []int
	RuntimeHealthChecked bool
	RuntimeHealthy       bool
}

func (s actualSandboxState) healthyRunning() bool {
	if len(s.ProcessPIDs) == 0 {
		return false
	}
	if s.RuntimeHealthChecked {
		return s.RuntimeHealthy
	}
	return false
}

func (s actualSandboxState) hasHostResidue() bool {
	return len(s.ProcessPIDs) > 0 || s.DockerDiskFDs > 0 || len(s.OpenPorts) > 0 || s.RuntimeHealthy
}

func (s actualSandboxState) fullyStopped() bool {
	return len(s.ProcessPIDs) == 0 && s.DockerDiskFDs == 0 && len(s.OpenPorts) == 0 && !s.RuntimeHealthy
}

func (m *MicrosandboxBackend) actualState(ctx context.Context, sandboxID string) actualSandboxState {
	state := actualSandboxState{ProcessPIDs: []int{}, OpenPorts: []int{}}
	if handle, err := microsandbox.GetSandbox(ctx, sandboxID); err == nil {
		state.NativeStatus = string(handle.Status())
	}
	state.ProcessPIDs = findMicrosandboxPIDs(sandboxID)
	state.DockerDiskFDs = countDockerDiskFDs(sandboxID)

	ports := m.sandboxPorts(sandboxID)
	for guestPort, hostPort := range ports {
		if tcpPortOpen(hostPort, actualProbeTimeout) {
			state.OpenPorts = append(state.OpenPorts, hostPort)
		}
		if guestPort == agentRuntimeGuestPort {
			state.RuntimeHealthChecked = true
			state.RuntimeHealthy = runtimeHealthOK(ctx, hostPort, actualProbeTimeout)
		}
	}
	return state
}

func (m *MicrosandboxBackend) waitForHealthyRunning(ctx context.Context, sandboxID string) error {
	return waitForCondition(ctx, startVerifyTimeout, func() (bool, string) {
		actual := m.actualState(ctx, sandboxID)
		return actual.healthyRunning(), fmt.Sprintf(
			"native=%s pids=%v disk_fds=%d open_ports=%v runtime_healthy=%v",
			actual.NativeStatus, actual.ProcessPIDs, actual.DockerDiskFDs, actual.OpenPorts, actual.RuntimeHealthy,
		)
	})
}

func (m *MicrosandboxBackend) waitForFullyStopped(ctx context.Context, sandboxID string) error {
	return waitForCondition(ctx, stopVerifyTimeout, func() (bool, string) {
		actual := m.actualState(ctx, sandboxID)
		return actual.fullyStopped(), fmt.Sprintf(
			"native=%s pids=%v disk_fds=%d open_ports=%v runtime_healthy=%v",
			actual.NativeStatus, actual.ProcessPIDs, actual.DockerDiskFDs, actual.OpenPorts, actual.RuntimeHealthy,
		)
	})
}

func waitForCondition(ctx context.Context, timeout time.Duration, check func() (bool, string)) error {
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		ok, details := check()
		last = details
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met within %s: %s", timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lifecyclePollInterval):
		}
	}
}

func (m *MicrosandboxBackend) forceStopActual(ctx context.Context, sandboxID string) error {
	pids := findMicrosandboxPIDs(sandboxID)
	if len(pids) == 0 {
		return nil
	}
	for _, pid := range pids {
		_ = signalProcess(pid, syscall.SIGTERM)
	}
	_ = waitForCondition(ctx, 5*time.Second, func() (bool, string) {
		pids := findMicrosandboxPIDs(sandboxID)
		return len(pids) == 0, fmt.Sprintf("pids=%v", pids)
	})
	for _, pid := range findMicrosandboxPIDs(sandboxID) {
		_ = signalProcess(pid, syscall.SIGKILL)
	}
	return nil
}

func signalProcess(pid int, sig syscall.Signal) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return syscall.Kill(pid, sig)
}

func findMicrosandboxPIDs(sandboxID string) []int {
	if runtime.GOOS != "linux" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if isMicrosandboxCommandFor(raw, sandboxID) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func isMicrosandboxCommandFor(raw []byte, sandboxID string) bool {
	args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
	if len(args) == 0 {
		return false
	}
	hasMSB := false
	hasSandbox := false
	hasName := false
	for i, arg := range args {
		base := filepath.Base(arg)
		if base == "msb" || strings.Contains(arg, "/msb") {
			hasMSB = true
		}
		if arg == "sandbox" {
			hasSandbox = true
		}
		if arg == "--name="+sandboxID {
			hasName = true
		}
		if arg == "--name" && i+1 < len(args) && args[i+1] == sandboxID {
			hasName = true
		}
	}
	return hasMSB && hasSandbox && hasName
}

func countDockerDiskFDs(sandboxID string) int {
	if runtime.GOOS != "linux" {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	diskPath := microsandboxVolumePath(dockerDataVolumeName(sandboxID))
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(target, diskPath) {
				count++
			}
		}
	}
	return count
}

func microsandboxVolumePath(volumeName string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/root"
	}
	return filepath.Join(home, ".microsandbox", "volumes", volumeName)
}

func tcpPortOpen(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func runtimeHealthOK(ctx context.Context, port int, timeout time.Duration) bool {
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func isAttachedDiskError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already attached") ||
		strings.Contains(message, "incompatible disk mode")
}

func ignoreNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") || strings.Contains(message, "does not exist") {
		return nil
	}
	return err
}

func ignoreAlreadyStopped(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "already stopped") ||
		strings.Contains(message, "not running") ||
		strings.Contains(message, "is stopped") {
		return nil
	}
	return err
}
