package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
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

func artifactWatchRun(path string) error {
	abs, err := resolveWatchDir(path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer cleanupOwnWatcherState(abs)
	return runArtifactWatchLoop(ctx, abs, watchInterval(), os.Stdout)
}

func runArtifactWatchLoop(ctx context.Context, artifactDir string, interval time.Duration, logOut io.Writer) error {
	logEvent := func(event string, fields map[string]any) {
		entry := map[string]any{
			"ts":            time.Now().UTC().Format(time.RFC3339),
			"event":         event,
			"artifact_path": artifactDir,
		}
		for key, value := range fields {
			entry[key] = value
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintln(logOut, string(data))
	}
	logEvent("watch_started", map[string]any{"interval_ms": interval.Milliseconds()})
	prev, err := snapshotArtifactDir(artifactDir)
	if err != nil {
		logEvent("watch_stopped", map[string]any{"error": err.Error()})
		return err
	}
	var lastHandled watchSnapshot
	var retryAt time.Time
	retryPending := false
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logEvent("watch_stopped", nil)
			return nil
		case <-ticker.C:
		}
		cur, err := snapshotArtifactDir(artifactDir)
		if err != nil {
			logEvent("watch_stopped", map[string]any{"error": err.Error()})
			return err
		}
		if !snapshotsEqual(cur, prev) {
			prev = cur
			continue
		}
		changed := !snapshotsEqual(cur, lastHandled)
		retryDue := retryPending && !time.Now().Before(retryAt)
		if !changed && !retryDue {
			continue
		}
		lastHandled = cur
		retryPending = false
		result, err := syncArtifactOnce(ctx, artifactDir)
		switch {
		case err == nil:
			logEvent("synced", map[string]any{"result": result})
		default:
			if validationErr, ok := err.(artifactValidationError); ok {
				logEvent("validation_failed", map[string]any{"issues": validationErr.Result.Issues})
			} else {
				retryPending = true
				retryAt = time.Now().Add(time.Duration(watchRetryTicks) * interval)
				logEvent("sync_failed", map[string]any{"error": err.Error()})
			}
		}
	}
}

func syncArtifactOnce(ctx context.Context, path string) (result map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	artifactDir, manifestPath, manifest, manifestObject, err := loadArtifact(path)
	if err != nil {
		return nil, err
	}
	if _, err := validateLoadedArtifact(artifactDir, manifestPath, manifest); err != nil {
		return nil, err
	}
	payload, err := buildArtifactSyncPayload(artifactDir, manifest, manifestObject)
	if err != nil {
		return nil, err
	}
	if err := postControlPlaneCtx(ctx, agentCanvasPath("artifacts/sync"), payload, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type watchFileStamp struct {
	ModTimeNanos int64
	Size         int64
}

type watchSnapshot map[string]watchFileStamp

func snapshotArtifactDir(dir string) (watchSnapshot, error) {
	snap := make(watchSnapshot)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) && path != dir {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		snap[rel] = watchFileStamp{ModTimeNanos: info.ModTime().UnixNano(), Size: info.Size()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

func snapshotsEqual(a, b watchSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for key, stamp := range a {
		if other, ok := b[key]; !ok || other != stamp {
			return false
		}
	}
	return true
}

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
