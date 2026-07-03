package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
)

// Size caps for the two publish artifacts (apps live-stream / publish plan).
// The bundle is the deployable build; source is the reproducible tree.
const maxAppNotesBytes = 8 << 10 // 8 KiB

// File size caps (vars so tests can shrink them without multi-hundred-MB
// fixtures).
var (
	maxAppBundleBytes int64 = 256 << 20 // 256 MiB
	maxAppSourceBytes int64 = 64 << 20  // 64 MiB
)

// Versions handles POST /v1/apps/{appID}/versions: a multipart upload of the
// source + bundle zips that streams both into immutable version storage and
// deploys synchronously. Auth matches the rest of /v1/apps (org session +
// channel authorization); API-key callers are rejected exactly like Launch —
// publishing is a human action.
// @Summary Publish a new app version
// @Description Streams the multipart `source` and `bundle` zips into immutable storage (server-computed sha256), records a version, and deploys it synchronously. Requires a user session; API-key callers are rejected.
// @Tags apps
// @Accept multipart/form-data
// @Produce json
// @Param appID path string true "App ID"
// @Param source formData file true "source.zip"
// @Param bundle formData file true "bundle.zip"
// @Param notes formData string false "Changelog for this version"
// @Success 201 {object} appPublishResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 413 {object} errorResponse
// @Security BearerAuth
// @Router /v1/apps/{appID}/versions [post]
func (h *AppsHandler) Versions(w http.ResponseWriter, r *http.Request) {
	org, app, ok := h.requireApp(w, r)
	if !ok {
		return
	}
	// Publishing is a human action; reject API-key callers like Launch does.
	user, hasUser := middleware.UserFromContext(r.Context())
	if !hasUser || user == nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "app publishing requires a user session"})
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "expected a multipart/form-data body"})
		return
	}

	var (
		sourceFile, bundleFile *os.File
		sourceSHA, bundleSHA   string
		notes                  string
	)
	cleanup := func() {
		for _, f := range []*os.File{sourceFile, bundleFile} {
			if f != nil {
				f.Close()
				os.Remove(f.Name())
			}
		}
	}
	defer cleanup()

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "malformed multipart body"})
			return
		}
		switch part.FormName() {
		case "source":
			sourceFile, sourceSHA, err = spoolPart(part, maxAppSourceBytes)
		case "bundle":
			bundleFile, bundleSHA, err = spoolPart(part, maxAppBundleBytes)
		case "notes":
			var b []byte
			b, err = io.ReadAll(io.LimitReader(part, maxAppNotesBytes))
			notes = string(b)
		}
		part.Close()
		if err != nil {
			if errors.Is(err, errPartTooLarge) {
				writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: part.FormName() + " exceeds the maximum allowed size"})
				return
			}
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "failed to read upload"})
			return
		}
	}

	if sourceFile == nil || bundleFile == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "both source and bundle files are required"})
		return
	}
	if _, err := sourceFile.Seek(0, io.SeekStart); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to stage upload"})
		return
	}
	if _, err := bundleFile.Seek(0, io.SeekStart); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to stage upload"})
		return
	}

	version, err := h.svc.PublishStream(r.Context(), apps.PublishStreamParams{
		OrgID:        org.ID,
		AppID:        app.ID,
		Source:       sourceFile,
		Bundle:       bundleFile,
		SourceSHA256: sourceSHA,
		BundleSHA256: bundleSHA,
		Notes:        notes,
	})
	if err != nil {
		if version != nil {
			// The version row exists but the deploy failed; the app is now in
			// status "failed". Surface the failure without discarding the row.
			logging.Capture(r.Context(), err)
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "version recorded but deploy failed: " + err.Error()})
			return
		}
		writeAppsError(w, r, err)
		return
	}

	reloaded, err := h.svc.GetApp(r.Context(), org.ID, app.ID)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}
	url, err := h.svc.AppURL(r.Context(), reloaded)
	if err != nil && !errors.Is(err, apps.ErrNotDeployed) {
		writeAppsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, appPublishResponse{
		VersionID: version.ID.String(),
		URL:       url,
		Status:    reloaded.Status,
	})
}

// appPublishResponse mirrors the MCP app_publish result shape.
type appPublishResponse struct {
	VersionID string `json:"version_id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
}

// errPartTooLarge is returned by spoolPart when a file part exceeds its cap.
var errPartTooLarge = errors.New("multipart file exceeds size limit")

// spoolPart streams one multipart file part to a temp file, computing its
// sha256 during the copy (server-authoritative — no client-supplied hash), and
// enforcing max. The caller owns closing/removing the returned file.
func spoolPart(part io.Reader, max int64) (*os.File, string, error) {
	f, err := os.CreateTemp("", "hivy-app-upload-*.zip")
	if err != nil {
		return nil, "", err
	}
	hasher := sha256.New()
	// max+1 so a file exactly at the cap passes but one byte over trips.
	n, err := io.Copy(io.MultiWriter(f, hasher), io.LimitReader(part, max+1))
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, "", err
	}
	if n > max {
		f.Close()
		os.Remove(f.Name())
		return nil, "", errPartTooLarge
	}
	return f, hex.EncodeToString(hasher.Sum(nil)), nil
}
