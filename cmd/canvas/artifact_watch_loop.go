package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

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
