package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var sha256Re = regexp.MustCompile(`^[a-f0-9]{64}$`)

type deployRequest struct {
	BundleURL string            `json:"bundle_url"`
	SHA256    string            `json:"sha256"`
	VersionID string            `json:"version_id"`
	Env       map[string]string `json:"env,omitempty"`
}

type deployResponse struct {
	OldSHA     string `json:"old_sha"`
	NewSHA     string `json:"new_sha"`
	VersionID  string `json:"version_id,omitempty"`
	DownloadMS int64  `json:"download_ms"`
	ExtractMS  int64  `json:"extract_ms"`
	RestartMS  int64  `json:"restart_ms"`
	TotalMS    int64  `json:"total_ms"`
}

// handleDeploy downloads a bundle, verifies its sha256, extracts it into a
// release directory, atomically swaps the current symlink, optionally writes
// the env file, and restarts the app.
//
//	POST /deploy {"bundle_url": "...", "sha256": "...", "version_id": "..."}
func (s *server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var req deployRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sha := strings.ToLower(strings.TrimSpace(req.SHA256))
	if !sha256Re.MatchString(sha) {
		writeError(w, http.StatusBadRequest, "sha256 must be 64 hex characters")
		return
	}
	if req.BundleURL == "" {
		writeError(w, http.StatusBadRequest, "bundle_url is required")
		return
	}

	s.deployMu.Lock()
	defer s.deployMu.Unlock()
	start := time.Now()

	downloadStart := time.Now()
	zipPath, err := s.downloadBundle(r, req.BundleURL, sha)
	if err != nil {
		s.log.ErrorContext(r.Context(), "bundle download failed", "error", err, "version_id", req.VersionID)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	defer os.Remove(zipPath)
	downloadMS := time.Since(downloadStart).Milliseconds()

	extractStart := time.Now()
	if err := s.stageRelease(zipPath, sha, req.VersionID); err != nil {
		s.log.ErrorContext(r.Context(), "bundle extraction failed", "error", err, "sha256", sha)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	extractMS := time.Since(extractStart).Milliseconds()

	if req.Env != nil {
		if err := writeEnvFile(s.cfg.envFile(), req.Env); err != nil {
			writeError(w, http.StatusBadRequest, "write env file: "+err.Error())
			return
		}
	}

	oldSHA := s.currentReleaseSHA()
	if err := swapCurrentSymlink(s.cfg, sha); err != nil {
		s.log.ErrorContext(r.Context(), "symlink swap failed", "error", err, "sha256", sha)
		writeError(w, http.StatusInternalServerError, "activate release: "+err.Error())
		return
	}

	restartStart := time.Now()
	if err := s.proc.Restart(r.Context()); err != nil {
		s.log.ErrorContext(r.Context(), "app restart failed after deploy", "error", err, "sha256", sha)
		writeError(w, http.StatusInternalServerError, "release activated but restart failed: "+err.Error())
		return
	}
	restartMS := time.Since(restartStart).Milliseconds()

	s.log.InfoContext(r.Context(), "deploy complete",
		"old_sha", oldSHA, "new_sha", sha, "version_id", req.VersionID,
		"total_ms", time.Since(start).Milliseconds())
	writeJSON(w, http.StatusOK, deployResponse{
		OldSHA:     oldSHA,
		NewSHA:     sha,
		VersionID:  req.VersionID,
		DownloadMS: downloadMS,
		ExtractMS:  extractMS,
		RestartMS:  restartMS,
		TotalMS:    time.Since(start).Milliseconds(),
	})
}

type rollbackRequest struct {
	SHA256 string `json:"sha256"`
}

// handleRollback swaps the current symlink to an already-extracted release
// and restarts the app.
//
//	POST /rollback {"sha256": "..."}
func (s *server) handleRollback(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sha := strings.ToLower(strings.TrimSpace(req.SHA256))
	if !sha256Re.MatchString(sha) {
		writeError(w, http.StatusBadRequest, "sha256 must be 64 hex characters")
		return
	}

	s.deployMu.Lock()
	defer s.deployMu.Unlock()

	releaseDir := filepath.Join(s.cfg.releasesDir(), sha)
	if info, err := os.Stat(filepath.Join(releaseDir, "server")); err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "release "+sha+" not found on disk")
		return
	}

	oldSHA := s.currentReleaseSHA()
	if err := swapCurrentSymlink(s.cfg, sha); err != nil {
		writeError(w, http.StatusInternalServerError, "activate release: "+err.Error())
		return
	}
	restartStart := time.Now()
	if err := s.proc.Restart(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "release activated but restart failed: "+err.Error())
		return
	}
	s.log.InfoContext(r.Context(), "rollback complete", "old_sha", oldSHA, "new_sha", sha)
	writeJSON(w, http.StatusOK, deployResponse{
		OldSHA:    oldSHA,
		NewSHA:    sha,
		RestartMS: time.Since(restartStart).Milliseconds(),
	})
}

// downloadBundle streams the bundle to a temp file under APP_ROOT/tmp while
// hashing it, and rejects size overruns and sha mismatches.
func (s *server) downloadBundle(r *http.Request, bundleURL, wantSHA string) (string, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, bundleURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid bundle_url: %w", err)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download bundle: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download bundle: unexpected status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(s.cfg.tmpDir(), "bundle-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp bundle: %w", err)
	}
	tmpPath := tmp.Name()
	fail := func(err error) (string, error) {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(resp.Body, maxBundleBytes+1))
	if err != nil {
		return fail(fmt.Errorf("download bundle: %w", err))
	}
	if written > maxBundleBytes {
		return fail(fmt.Errorf("bundle exceeds max size of %d bytes", int64(maxBundleBytes)))
	}
	if err := tmp.Close(); err != nil {
		return fail(err)
	}
	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if gotSHA != wantSHA {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sha256 mismatch: got %s want %s", gotSHA, wantSHA)
	}
	return tmpPath, nil
}

// stageRelease extracts the verified bundle into releases/{sha}. Extraction
// happens in a temp staging dir that is renamed into place, so a partially
// extracted release is never observable. Existing release dirs are reused
// (content is immutable by sha).
func (s *server) stageRelease(zipPath, sha, versionID string) error {
	releaseDir := filepath.Join(s.cfg.releasesDir(), sha)
	if _, err := os.Stat(filepath.Join(releaseDir, "server")); err == nil {
		return nil
	}

	staging, err := os.MkdirTemp(s.cfg.tmpDir(), "release-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := extractZip(zipPath, staging); err != nil {
		return err
	}
	serverBin := filepath.Join(staging, "server")
	info, err := os.Stat(serverBin)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("bundle has no server binary at its root")
	}
	if err := os.Chmod(serverBin, 0o755); err != nil {
		return fmt.Errorf("chmod server binary: %w", err)
	}
	if versionID != "" {
		if err := os.WriteFile(filepath.Join(staging, ".hivy_version_id"), []byte(versionID+"\n"), 0o600); err != nil {
			return fmt.Errorf("write version marker: %w", err)
		}
	}
	if err := os.RemoveAll(releaseDir); err != nil {
		return fmt.Errorf("clear stale release dir: %w", err)
	}
	if err := os.Rename(staging, releaseDir); err != nil {
		return fmt.Errorf("finalize release dir: %w", err)
	}
	return nil
}

// swapCurrentSymlink atomically repoints APP_ROOT/current at releases/{sha}
// via a temp symlink + rename (rename over an existing symlink is atomic).
func swapCurrentSymlink(cfg config, sha string) error {
	tmpLink := filepath.Join(cfg.AppRoot, ".current-"+sha[:12])
	_ = os.Remove(tmpLink)
	if err := os.Symlink(filepath.Join("releases", sha), tmpLink); err != nil {
		return err
	}
	if err := os.Rename(tmpLink, cfg.currentLink()); err != nil {
		_ = os.Remove(tmpLink)
		return err
	}
	return nil
}

// currentReleaseSHA reads the sha of the active release from the current
// symlink; empty when nothing is deployed.
func (s *server) currentReleaseSHA() string {
	target, err := os.Readlink(s.cfg.currentLink())
	if err != nil {
		return ""
	}
	sha := filepath.Base(target)
	if sha256Re.MatchString(sha) {
		return sha
	}
	return ""
}
