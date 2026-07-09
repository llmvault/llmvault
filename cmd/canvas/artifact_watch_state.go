package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func watcherInfo(state watcherState) map[string]any {
	return map[string]any{
		"watching":      true,
		"pid":           state.PID,
		"artifact_path": state.ArtifactPath,
		"log_path":      state.LogPath,
		"started_at":    state.StartedAt,
		"auto_sync":     "file changes are validated and synced to Canvas automatically",
		"stop_command":  fmt.Sprintf("%s artifact watch stop %s", cliName(), state.ArtifactPath),
	}
}

func resolveWatchDir(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return "", errors.New("artifact path is required")
	}
	if filepath.Base(clean) == "artifact.json" {
		clean = filepath.Dir(clean)
	}
	return filepath.Abs(clean)
}

func watchersDir() string {
	return filepath.Join(stateDir(), "watchers")
}

func watcherKey(abs string) string {
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:6])
}

func watcherStatePath(abs string) string {
	return filepath.Join(watchersDir(), watcherKey(abs)+".json")
}

func watcherLogPath(abs string) string {
	return filepath.Join(watchersDir(), watcherKey(abs)+".log")
}

func readWatcherState(abs string) (watcherState, bool, error) {
	var state watcherState
	data, err := os.ReadFile(watcherStatePath(abs)) // #nosec G304 -- path derived from state dir.
	if err != nil {
		if os.IsNotExist(err) {
			return state, false, nil
		}
		return state, false, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, false, err
	}
	return state, true, nil
}

func writeWatcherState(abs string, state watcherState) error {
	if err := os.MkdirAll(watchersDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(watcherStatePath(abs), append(data, '\n'), 0o600)
}

func removeWatcherState(abs string) {
	_ = os.Remove(watcherStatePath(abs))
}

func cleanupOwnWatcherState(abs string) {
	if state, ok, _ := readWatcherState(abs); ok && state.PID == os.Getpid() {
		removeWatcherState(abs)
	}
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func canvasRuntimeConfigured() bool {
	for _, key := range []string{envControlPlaneURL, envAgentID, envRuntimeSecret} {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return false
		}
	}
	return true
}

func watchInterval() time.Duration {
	if raw := strings.TrimSpace(os.Getenv(envWatchIntervalMS)); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 50 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultWatchInterval
}
