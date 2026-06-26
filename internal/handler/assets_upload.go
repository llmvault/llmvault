package handler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

const maxDriveAssetUploadBytes = 25 * 1024 * 1024

var driveAssetTypes = map[string]bool{
	"audio/m4a":   true,
	"audio/mp3":   true,
	"audio/mp4":   true,
	"audio/mpeg":  true,
	"audio/ogg":   true,
	"audio/wav":   true,
	"audio/webm":  true,
	"audio/x-m4a": true,
	"audio/x-wav": true,
	"image/gif":   true,
	"image/jpeg":  true,
	"image/png":   true,
	"image/webp":  true,
	"video/mp4":   true,
	"video/webm":  true,
}

// UploadAgentAsset handles POST /v1/assets/upload.
//
// The request is multipart/form-data with:
//   - file: image or audio file
//   - agent_id: destination agent
//   - path: optional drive folder, default "uploads"
//
// @Summary Upload agent drive asset
// @Description Uploads an image, audio, or supported video file into an agent drive folder.
// @Tags assets
// @Accept mpfd
// @Produce json
// @Param agent_id formData string true "Destination agent ID"
// @Param path formData string false "Drive folder"
// @Param file formData file true "Asset file"
// @Success 201 {object} streamAssetResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/assets/upload [post]
func (h *UploadsHandler) UploadAgentAsset(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset storage unavailable"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxDriveAssetUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxDriveAssetUploadBytes + (1 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	agentID, err := uuid.Parse(strings.TrimSpace(r.FormValue("agent_id")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	var agent model.Agent
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return
	}

	folder := strings.TrimSpace(r.FormValue("path"))
	if folder == "" {
		folder = "uploads"
	}
	folder, err = validateFolder(folder)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	if header.Size > maxDriveAssetUploadBytes {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "file exceeds maximum size"})
		return
	}
	filename := cleanUploadFilename(header.Filename)
	if filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filename is required"})
		return
	}

	contentType, body, err := sniffDriveAssetUpload(file, header.Header.Get("Content-Type"))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	sandbox, err := h.agentRuntimeSelector().MainRuntime(r.Context(), org.ID, agentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "agent runtime sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve agent runtime"})
		return
	}

	assetID := uuid.New()
	storedName := assetID.String() + "-" + filename
	key := buildAgentAssetKey(agentID, folder, storedName)
	stored, err := h.streamer.Stream(r.Context(), key, contentType, &uploadLimitReader{
		r: body,
		n: maxDriveAssetUploadBytes + 1,
	})
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "file exceeds maximum size"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "drive image upload failed",
			"agent_id", agentID,
			"org_id", org.ID,
			"key", key,
			"error", err,
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "upload failed"})
		return
	}
	if stored.Bytes > maxDriveAssetUploadBytes {
		_ = h.streamer.Delete(r.Context(), key)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "file exceeds maximum size"})
		return
	}

	now := time.Now()
	asset := model.AgentAsset{
		ID:          assetID,
		OrgID:       org.ID,
		AgentID:     agentID,
		SandboxID:   sandbox.ID,
		Path:        folder,
		Filename:    filename,
		Key:         key,
		PublicURL:   h.publicAssetURL(key, ""),
		ContentType: contentType,
		Bytes:       stored.Bytes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.db.WithContext(r.Context()).Create(&asset).Error; err != nil {
		_ = h.streamer.Delete(r.Context(), key)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create asset record"})
		return
	}

	writeJSON(w, http.StatusCreated, streamAssetResponse{
		ID:          asset.ID,
		PublicURL:   h.publicAssetURL(asset.Key, asset.PublicURL),
		Key:         asset.Key,
		Path:        asset.Path,
		Filename:    asset.Filename,
		ContentType: asset.ContentType,
		Bytes:       asset.Bytes,
	})
}

func cleanUploadFilename(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return ""
	}
	name := path.Base(raw)
	if name == "." || name == "/" || name == "" {
		return ""
	}
	if strings.ContainsRune(name, 0) {
		return ""
	}
	return name
}

func sniffDriveAssetUpload(file io.Reader, headerType string) (string, io.Reader, error) {
	var head [512]byte
	n, err := io.ReadFull(file, head[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", nil, fmt.Errorf("failed to read file")
	}
	prefix := head[:n]
	detected := http.DetectContentType(prefix)
	headerType = strings.ToLower(strings.TrimSpace(strings.Split(headerType, ";")[0]))
	if driveAssetTypes[headerType] {
		return headerType, io.MultiReader(bytes.NewReader(prefix), file), nil
	}
	if driveAssetTypes[detected] {
		return detected, io.MultiReader(bytes.NewReader(prefix), file), nil
	}
	return "", nil, fmt.Errorf("content_type must be an image or audio file")
}

var errUploadTooLarge = errors.New("upload too large")

type uploadLimitReader struct {
	r io.Reader
	n int64
}

func (l *uploadLimitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errUploadTooLarge
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	if l.n <= 0 && err == nil {
		return n, errUploadTooLarge
	}
	return n, err
}
