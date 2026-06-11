package handler

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/storage"
)

// uploadProxyHTTPClient is a shared client with a bounded timeout for proxying
// uploads to S3. http.DefaultClient has no timeout, so a stalled S3 endpoint
// would hold the request goroutine (and the buffered file body) open forever.
var uploadProxyHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Upload handles POST /v1/uploads/upload — a server-side upload that accepts
// the file directly and forwards it to S3 via the presigned URL. This works
// in environments where the browser cannot reach the S3 endpoint directly
// (e.g. local dev with Docker-hosted MinIO).
func (h *UploadsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}
	org, _ := middleware.OrgFromContext(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	assetType := storage.AssetType(r.FormValue("asset_type"))
	if assetType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "asset_type is required"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	policy, ok := h.presigner.Policy(assetType)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown asset_type"})
		return
	}
	if _, ok := policy.AllowedTypes[contentType]; !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "content_type not allowed for asset_type"})
		return
	}

	var orgID *uuid.UUID
	if assetType == storage.AssetTypeOrgLogo {
		oid, err := resolveOrgLogoOrg(strPtr(r.FormValue("org_id")), org)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.requireOrgMembership(r, user.ID, oid); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		orgID = &oid
	}

	signReq := storage.SignRequest{
		AssetType:   assetType,
		UserID:      user.ID,
		OrgID:       orgID,
		ContentType: contentType,
		SizeBytes:   header.Size,
		Filename:    header.Filename,
	}

	out, err := h.presigner.Sign(r.Context(), signReq)
	if err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "upload sign rejected", "user_id", user.ID, "asset_type", assetType, "error", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	body, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
		return
	}

	putReq, err := http.NewRequestWithContext(r.Context(), out.UploadMethod, out.UploadURL, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upload request"})
		return
	}
	for k, v := range out.RequiredHeaders {
		putReq.Header.Set(k, v)
	}
	putReq.ContentLength = int64(len(body))

	resp, err := uploadProxyHTTPClient.Do(putReq)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "upload proxy failed", "user_id", user.ID, "asset_type", assetType, "key", out.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "upload failed"})
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "upload proxy returned error", "user_id", user.ID, "asset_type", assetType, "key", out.Key, "status", resp.StatusCode)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "upload failed"})
		return
	}

	writeJSON(w, http.StatusOK, signUploadResponse{
		UploadURL:    out.UploadURL,
		UploadMethod: out.UploadMethod,
		Key:          out.Key,
		PublicURL:    h.publicAssetURL(out.Key, ""),
		ExpiresAt:    out.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		MaxSizeBytes: out.MaxSizeBytes,
	})
}

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
