package handler

import (
	"archive/zip"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
)

// defaultAppsTemplateDir is the in-repo app template, resolved relative to
// the server working directory like the other global assets
// (NewAutomationCatalogHandler uses "global/triggers" the same way).
const defaultAppsTemplateDir = "global/apps/template"

// appsTemplateZipExcludes are directory names never shipped to builders:
// build outputs and dependency caches are rebuilt in the sandbox.
var appsTemplateZipExcludes = map[string]bool{
	"node_modules": true,
	"dist":         true,
	"public":       true,
	".git":         true,
}

// WithAppsTemplateDir overrides the app template directory (tests).
func (h *UploadsHandler) WithAppsTemplateDir(dir string) *UploadsHandler {
	h.appsTemplateDir = dir
	return h
}

func (h *UploadsHandler) resolvedAppsTemplateDir() string {
	if strings.TrimSpace(h.appsTemplateDir) != "" {
		return h.appsTemplateDir
	}
	return defaultAppsTemplateDir
}

// StreamAppsTemplateZip streams a zip of the app template, built at request
// time, to a builder agent. Auth is the existing sandbox runtime-secret
// bearer (authAgent) — builders derive this URL from HIVY_DRIVE_UPLOAD_URL by
// replacing the /drive suffix with /apps-template.zip.
//
//	GET /internal/agents/{agentID}/sandboxes/{sandboxID}/apps-template.zip
func (h *UploadsHandler) StreamAppsTemplateZip(w http.ResponseWriter, r *http.Request) {
	agent, _, ok := h.authAgent(w, r)
	if !ok {
		return
	}

	root := h.resolvedAppsTemplateDir()
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app template is not available"})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="apps-template.zip"`)
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && appsTemplateZipExcludes[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil // no symlinks or devices in the archive
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		dst, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(dst, src)
		return err
	})
	if err != nil {
		// Headers are already flushed — closing the writer mid-archive leaves
		// the client with a truncated zip, which is the correct failure signal.
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "apps template zip stream failed",
			"agent_id", agent.ID, "error", err)
	}
	if err := zw.Close(); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "apps template zip close failed",
			"agent_id", agent.ID, "error", err)
	}
}
