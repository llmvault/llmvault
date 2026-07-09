package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	envWatchIntervalMS   = "CANVAS_WATCH_INTERVAL_MS"
	defaultWatchInterval = 500 * time.Millisecond
	watchRetryTicks      = 10
	watchStopTimeout     = 2 * time.Second
)

type watcherState struct {
	PID          int    `json:"pid"`
	ArtifactPath string `json:"artifact_path"`
	LogPath      string `json:"log_path"`
	StartedAt    string `json:"started_at"`
}

func artifactWatchCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: artifact watch <artifact-path> | artifact watch stop <artifact-path> | artifact watch status [<artifact-path>]")
	}
	switch args[0] {
	case "stop":
		if len(args) != 2 {
			return errors.New("artifact path is required")
		}
		result, err := stopArtifactWatcher(args[1])
		if err != nil {
			return err
		}
		return printJSON(result)
	case "status":
		return artifactWatchStatusCommand(args[1:])
	case "run":
		if len(args) != 2 {
			return errors.New("artifact path is required")
		}
		return artifactWatchRun(args[1])
	default:
		if len(args) != 1 {
			return errors.New("artifact path is required")
		}
		result, err := startArtifactWatcher(args[0])
		if err != nil {
			return err
		}
		return printJSON(result)
	}
}

func startArtifactWatcher(path string) (map[string]any, error) {
	if !canvasRuntimeConfigured() {
		return nil, fmt.Errorf("canvas runtime not configured: watch requires %s, %s, and %s", envControlPlaneURL, envAgentID, envRuntimeSecret)
	}
	artifactDir, _, _, _, err := loadArtifact(path)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(artifactDir)
	if err != nil {
		return nil, err
	}
	if state, ok, _ := readWatcherState(abs); ok && pidAlive(state.PID) {
		result := watcherInfo(state)
		result["already_watching"] = true
		return result, nil
	}
	logPath := watcherLogPath(abs)
	if err := os.MkdirAll(watchersDir(), 0o700); err != nil {
		return nil, err
	}
	pid, err := watchSpawn(abs, logPath)
	if err != nil {
		return nil, err
	}
	state := watcherState{
		PID:          pid,
		ArtifactPath: abs,
		LogPath:      logPath,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeWatcherState(abs, state); err != nil {
		return nil, err
	}
	result := watcherInfo(state)
	result["already_watching"] = false
	return result, nil
}

var watchSpawn = func(artifactDir, logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path derived from state dir.
	if err != nil {
		return 0, err
	}
	defer func() { _ = logFile.Close() }()
	cmd := exec.Command(exe, "artifact", "watch", "run", artifactDir) // #nosec G204 -- re-executes this binary.
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

func stopArtifactWatcher(path string) (map[string]any, error) {
	abs, err := resolveWatchDir(path)
	if err != nil {
		return nil, err
	}
	state, ok, err := readWatcherState(abs)
	if err != nil {
		return nil, err
	}
	if !ok || !pidAlive(state.PID) {
		removeWatcherState(abs)
		return map[string]any{
			"watching":      false,
			"stopped":       false,
			"artifact_path": abs,
			"message":       "no active watcher for this artifact",
		}, nil
	}
	_ = syscall.Kill(state.PID, syscall.SIGTERM)
	deadline := time.Now().Add(watchStopTimeout)
	for time.Now().Before(deadline) && pidAlive(state.PID) {
		time.Sleep(50 * time.Millisecond)
	}
	if pidAlive(state.PID) {
		_ = syscall.Kill(state.PID, syscall.SIGKILL)
	}
	removeWatcherState(abs)
	return map[string]any{
		"watching":      false,
		"stopped":       true,
		"pid":           state.PID,
		"artifact_path": state.ArtifactPath,
	}, nil
}

func artifactWatchStatusCommand(args []string) error {
	if len(args) > 1 {
		return errors.New("artifact watch status accepts at most one artifact path")
	}
	if len(args) == 1 {
		abs, err := resolveWatchDir(args[0])
		if err != nil {
			return err
		}
		state, ok, err := readWatcherState(abs)
		if err != nil {
			return err
		}
		if !ok || !pidAlive(state.PID) {
			if ok {
				removeWatcherState(abs)
			}
			return printJSON(map[string]any{"watching": false, "artifact_path": abs})
		}
		return printJSON(watcherInfo(state))
	}
	entries, err := os.ReadDir(watchersDir())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	watchers := []map[string]any{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		statePath := filepath.Join(watchersDir(), entry.Name())
		data, err := os.ReadFile(statePath) // #nosec G304 -- path derived from state dir.
		if err != nil {
			continue
		}
		var state watcherState
		if json.Unmarshal(data, &state) != nil {
			continue
		}
		if !pidAlive(state.PID) {
			_ = os.Remove(statePath)
			continue
		}
		watchers = append(watchers, watcherInfo(state))
	}
	return printJSON(map[string]any{"watchers": watchers})
}
