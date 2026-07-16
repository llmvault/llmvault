package runner

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	actualProbeTimeout    = 750 * time.Millisecond
	stopVerifyTimeout     = 30 * time.Second
	startVerifyTimeout    = 90 * time.Second
	lifecyclePollInterval = 250 * time.Millisecond
)

type actualSandboxState struct {
	NativeStatus  string
	ProcessPIDs   []int
	VolumeDiskFDs int
	OpenPorts     []int
}

func (s actualSandboxState) infrastructureRunning() bool {
	nativeRunning := strings.EqualFold(s.NativeStatus, "running")
	if runtime.GOOS != "linux" {
		return nativeRunning
	}
	return nativeRunning && len(s.ProcessPIDs) > 0
}

func (s actualSandboxState) hasHostResidue() bool {
	return len(s.ProcessPIDs) > 0 || s.VolumeDiskFDs > 0 || len(s.OpenPorts) > 0
}

func (s actualSandboxState) fullyStopped() bool {
	return s.runtimeStopped() && s.VolumeDiskFDs == 0
}

// runtimeStopped is the routing boundary: once no VM process or preview port
// remains, callers must stop advertising the sandbox even if volume cleanup is
// still settling in the background.
func (s actualSandboxState) runtimeStopped() bool {
	return !strings.EqualFold(s.NativeStatus, "running") && len(s.ProcessPIDs) == 0 && len(s.OpenPorts) == 0
}

func (m *MicrosandboxBackend) actualState(ctx context.Context, sandboxID string) actualSandboxState {
	state := actualSandboxState{ProcessPIDs: []int{}, OpenPorts: []int{}}
	if handle, err := microsandbox.GetSandbox(ctx, sandboxID); err == nil {
		state.NativeStatus = string(handle.Status())
	}
	state.ProcessPIDs = findMicrosandboxPIDs(sandboxID)
	state.VolumeDiskFDs = countSandboxVolumeFDs(sandboxID)

	ports := m.sandboxPorts(sandboxID)
	for _, hostPort := range ports {
		if tcpPortOpen(hostPort, actualProbeTimeout) {
			state.OpenPorts = append(state.OpenPorts, hostPort)
		}
	}
	return state
}

func (m *MicrosandboxBackend) waitForInfrastructureRunning(ctx context.Context, sandboxID string) error {
	return waitForCondition(ctx, startVerifyTimeout, func() (bool, string) {
		actual := m.actualState(ctx, sandboxID)
		return actual.infrastructureRunning(), fmt.Sprintf(
			"native=%s pids=%v disk_fds=%d open_ports=%v",
			actual.NativeStatus, actual.ProcessPIDs, actual.VolumeDiskFDs, actual.OpenPorts,
		)
	})
}

func (m *MicrosandboxBackend) waitForFullyStopped(ctx context.Context, sandboxID string) error {
	return waitForCondition(ctx, stopVerifyTimeout, func() (bool, string) {
		actual := m.actualState(ctx, sandboxID)
		return actual.fullyStopped(), fmt.Sprintf(
			"native=%s pids=%v disk_fds=%d open_ports=%v",
			actual.NativeStatus, actual.ProcessPIDs, actual.VolumeDiskFDs, actual.OpenPorts,
		)
	})
}

func (m *MicrosandboxBackend) waitForRuntimeStopped(ctx context.Context, sandboxID string) error {
	return waitForCondition(ctx, stopVerifyTimeout, func() (bool, string) {
		actual := m.actualState(ctx, sandboxID)
		return actual.runtimeStopped(), fmt.Sprintf(
			"native=%s pids=%v disk_fds=%d open_ports=%v",
			actual.NativeStatus, actual.ProcessPIDs, actual.VolumeDiskFDs, actual.OpenPorts,
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

func tcpPortOpen(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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
