package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// appStatus describes the app process as seen by the active process manager.
type appStatus struct {
	Mode      string `json:"mode"`
	State     string `json:"state"`
	Detail    string `json:"detail,omitempty"`
	PID       int    `json:"pid,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

const (
	stateRunning     = "running"
	stateStopped     = "stopped"
	stateNotDeployed = "not-deployed"
	stateFailed      = "failed"
)

// processManager abstracts control of the app process. Two implementations
// exist: systemctl-backed (microsandbox, where systemd is PID1) and direct
// child-process supervision (docker provider, where the image entrypoint runs
// appd without booting systemd).
type processManager interface {
	// Mode identifies the implementation ("systemd" or "direct").
	Mode() string
	// Start brings the app up if a release is deployed; it tolerates the
	// not-yet-deployed state without error.
	Start(ctx context.Context) error
	// Restart stops and starts the app (deploy/rollback/env changes).
	Restart(ctx context.Context) error
	// Status reports the current app process state.
	Status(ctx context.Context) appStatus
	// Journal returns a recent unit journal excerpt, or nil when the
	// implementation has no journal (direct mode).
	Journal(ctx context.Context, lines int) []string
	// Shutdown releases resources and stops supervision (not the unit).
	Shutdown(ctx context.Context)
}

// detectProcessManager picks the systemd implementation when systemd is
// actually booted as PID1 (the sd_booted(3) check: /run/systemd/system
// exists), otherwise falls back to direct supervision.
func detectProcessManager(cfg config, logger *slog.Logger) processManager {
	if systemdBooted() {
		return newSystemdManager(cfg, logger)
	}
	return newDirectManager(cfg, logger)
}

func systemdBooted() bool {
	info, err := os.Stat("/run/systemd/system")
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = exec.LookPath("systemctl")
	return err == nil
}

// currentServerBinary returns the path to the deployed server binary and
// whether it exists as an executable regular file.
func currentServerBinary(cfg config) (string, bool) {
	bin := filepath.Join(cfg.currentLink(), "server")
	info, err := os.Stat(bin)
	if err != nil || !info.Mode().IsRegular() {
		return bin, false
	}
	return bin, true
}
