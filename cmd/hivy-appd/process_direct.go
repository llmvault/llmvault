package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/usehivy/hivy/internal/goroutine"
)

// directManager supervises the app as a direct child process. Used when
// systemd is not PID1 (the docker provider), mirroring hivy-app.service:
// it runs APP_ROOT/current/server with the env file, appends stdout/stderr
// to LOGS_DIR/app.log, and restarts on failure with backoff.
type directManager struct {
	cfg config
	log *slog.Logger

	mu        chan struct{} // 1-buffered semaphore used as a mutex
	gen       uint64        // bumped on every intentional stop; stale monitors bail
	cmd       *exec.Cmd
	done      chan struct{} // closed when the current process exits
	startedAt time.Time
	failures  int
	lastErr   string
	stopped   bool
}

func newDirectManager(cfg config, logger *slog.Logger) *directManager {
	m := &directManager{cfg: cfg, log: logger, mu: make(chan struct{}, 1)}
	m.mu <- struct{}{}
	return m
}

func (m *directManager) lock()   { <-m.mu }
func (m *directManager) unlock() { m.mu <- struct{}{} }

func (m *directManager) Mode() string { return "direct" }

func (m *directManager) Start(ctx context.Context) error {
	m.lock()
	defer m.unlock()
	if m.cmd != nil {
		return nil
	}
	return m.startLocked(ctx)
}

func (m *directManager) Restart(ctx context.Context) error {
	m.lock()
	m.gen++
	m.failures = 0
	cmd, done := m.cmd, m.done
	m.cmd, m.done = nil, nil
	m.unlock()
	if cmd != nil {
		stopProcess(ctx, m.log, cmd, done)
	}
	m.lock()
	defer m.unlock()
	return m.startLocked(ctx)
}

func (m *directManager) Status(context.Context) appStatus {
	m.lock()
	defer m.unlock()
	status := appStatus{Mode: "direct", State: stateStopped}
	if m.cmd != nil && m.cmd.Process != nil {
		status.State = stateRunning
		status.PID = m.cmd.Process.Pid
		status.StartedAt = m.startedAt.UTC().Format(time.RFC3339)
		return status
	}
	if _, ok := currentServerBinary(m.cfg); !ok {
		status.State = stateNotDeployed
		return status
	}
	if m.lastErr != "" {
		status.State = stateFailed
		status.Detail = m.lastErr
	}
	return status
}

func (m *directManager) Journal(context.Context, int) []string { return nil }

func (m *directManager) Shutdown(ctx context.Context) {
	m.lock()
	m.stopped = true
	m.gen++
	cmd, done := m.cmd, m.done
	m.cmd, m.done = nil, nil
	m.unlock()
	if cmd != nil {
		stopProcess(ctx, m.log, cmd, done)
	}
}

// startLocked launches APP_ROOT/current/server. Caller holds the lock.
// The not-yet-deployed state is tolerated: it records status and returns nil,
// exactly like hivy-app.service's ConditionPathExists skip.
func (m *directManager) startLocked(ctx context.Context) error {
	bin, ok := currentServerBinary(m.cfg)
	if !ok {
		m.lastErr = ""
		m.log.InfoContext(ctx, "app not deployed yet; nothing to start", "path", bin)
		return nil
	}
	envVars, err := parseEnvFile(m.cfg.envFile())
	if err != nil {
		return fmt.Errorf("read env file: %w", err)
	}
	logFile, err := os.OpenFile(m.cfg.appLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open app log: %w", err)
	}

	cmd := exec.Command(bin)
	cmd.Dir = filepath.Dir(bin)
	cmd.Env = appendBaseEnv(envVars)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		m.lastErr = "start failed: " + err.Error()
		return fmt.Errorf("start app: %w", err)
	}

	m.gen++
	gen := m.gen
	done := make(chan struct{})
	m.cmd, m.done, m.startedAt, m.lastErr = cmd, done, time.Now(), ""
	m.log.InfoContext(ctx, "app started", "pid", cmd.Process.Pid, "binary", bin)

	goroutine.Go(context.WithoutCancel(ctx), func(ctx context.Context) {
		m.supervise(ctx, gen, cmd, done, logFile)
	})
	return nil
}

// supervise waits for the process to exit and, when the exit was not caused
// by an intentional stop, restarts it with exponential backoff (parity with
// hivy-app.service's Restart=on-failure).
func (m *directManager) supervise(ctx context.Context, gen uint64, cmd *exec.Cmd, done chan struct{}, logFile *os.File) {
	waitErr := cmd.Wait()
	_ = logFile.Close()
	close(done)

	m.lock()
	if m.stopped || gen != m.gen {
		m.unlock()
		return
	}
	m.cmd, m.done = nil, nil
	if time.Since(m.startedAt) > time.Minute {
		m.failures = 0
	}
	m.failures++
	m.lastErr = exitDetail(waitErr)
	delay := restartBackoff(m.failures)
	m.unlock()

	m.log.WarnContext(ctx, "app exited unexpectedly; restarting",
		"error", m.lastErr, "failures", m.failures, "backoff", delay.String())
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}

	m.lock()
	defer m.unlock()
	if m.stopped || gen != m.gen || m.cmd != nil {
		return
	}
	if err := m.startLocked(ctx); err != nil {
		m.log.ErrorContext(ctx, "app restart failed", "error", err)
	}
}

// stopProcess terminates the process group: SIGTERM, grace period, SIGKILL.
func stopProcess(ctx context.Context, logger *slog.Logger, cmd *exec.Cmd, done chan struct{}) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	signalGroup(pid, syscall.SIGTERM)
	select {
	case <-done:
		return
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
	}
	logger.WarnContext(ctx, "app did not stop after SIGTERM; killing", "pid", pid)
	signalGroup(pid, syscall.SIGKILL)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		logger.ErrorContext(ctx, "app process did not exit after SIGKILL", "pid", pid)
	}
}

// signalGroup signals the whole process group (Setpgid puts the app in its
// own group), falling back to the single process.
func signalGroup(pid int, sig syscall.Signal) {
	if err := syscall.Kill(-pid, sig); err != nil {
		_ = syscall.Kill(pid, sig)
	}
}

func restartBackoff(failures int) time.Duration {
	delay := time.Second
	for i := 1; i < failures && delay < 30*time.Second; i++ {
		delay *= 2
	}
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}

func exitDetail(err error) string {
	if err == nil {
		return "exited cleanly"
	}
	return err.Error()
}

// appendBaseEnv provides the minimal base environment the app gets under
// systemd (PATH and HOME), plus the env-file variables.
func appendBaseEnv(vars []string) []string {
	base := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/workspace",
	}
	return append(base, vars...)
}
