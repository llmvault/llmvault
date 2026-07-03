package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// systemdManager drives the app through systemctl. Used when systemd is PID1
// (microsandbox), where hivy-app.service runs the deployed server binary.
type systemdManager struct {
	cfg  config
	log  *slog.Logger
	unit string
}

func newSystemdManager(cfg config, logger *slog.Logger) *systemdManager {
	return &systemdManager{cfg: cfg, log: logger, unit: appUnitName}
}

func (m *systemdManager) Mode() string { return "systemd" }

// Start is a no-op: systemd starts the enabled unit at boot, and the unit's
// ConditionPathExists makes the not-yet-deployed state a clean skip.
func (m *systemdManager) Start(context.Context) error { return nil }

func (m *systemdManager) Restart(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := m.systemctl(ctx, "restart", m.unit)
	if err != nil {
		return fmt.Errorf("systemctl restart %s: %w: %s", m.unit, err, out)
	}
	return nil
}

func (m *systemdManager) Status(ctx context.Context) appStatus {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	status := appStatus{Mode: "systemd", State: stateStopped}
	if _, ok := currentServerBinary(m.cfg); !ok {
		status.State = stateNotDeployed
	}
	out, err := m.systemctl(ctx, "show", m.unit,
		"--property=ActiveState,SubState,ExecMainPID,ExecMainStartTimestamp", "--no-pager")
	if err != nil {
		status.Detail = "systemctl show failed: " + err.Error()
		return status
	}
	props := parseSystemctlShow(out)
	switch props["ActiveState"] {
	case "active":
		status.State = stateRunning
	case "failed":
		status.State = stateFailed
	}
	if sub := props["SubState"]; sub != "" {
		status.Detail = props["ActiveState"] + "/" + sub
	}
	if pid, err := strconv.Atoi(props["ExecMainPID"]); err == nil && pid > 0 {
		status.PID = pid
	}
	if ts := props["ExecMainStartTimestamp"]; ts != "" && ts != "n/a" {
		status.StartedAt = ts
	}
	return status
}

func (m *systemdManager) Journal(ctx context.Context, lines int) []string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if lines <= 0 {
		lines = 100
	}
	// #nosec G204 -- fixed binary; unit is a package constant, lines is an int.
	cmd := exec.CommandContext(ctx, "journalctl", "-u", m.unit,
		"-n", strconv.Itoa(lines), "--no-pager", "--output=short-iso")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		m.log.WarnContext(ctx, "journalctl failed", "unit", m.unit, "error", err)
		return nil
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func (m *systemdManager) Shutdown(context.Context) {}

func (m *systemdManager) systemctl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

func parseSystemctlShow(out string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			props[key] = value
		}
	}
	return props
}
