package handler

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
)

var allowedPublicAssetPrefixes = []string{
	"avatars/",
	"pub/o/",
	"pub/u/",
	"pub/e/",
}

// PreviewAsset redirects a stable Hivy asset URL to a short-lived signed S3 URL.
//
// @Summary Preview public asset
// @Description Redirects to a short-lived signed URL for an object in the public assets bucket.
// @Tags assets
// @Param path query string true "Public assets S3 key"
// @Success 302
// @Failure 400 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /v1/assets/preview [get]
func (h *UploadsHandler) PreviewAsset(w http.ResponseWriter, r *http.Request) {
	if h.presigner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset preview is not configured"})
		return
	}

	key, err := normalizePublicAssetKey(r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	location, err := h.presigner.PresignGet(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to sign asset URL"})
		return
	}

	w.Header().Set("Cache-Control", "private, no-store")
	http.Redirect(w, r, location, http.StatusFound)
}

func (h *UploadsHandler) publicAssetURL(key, fallback string) string {
	if key == "" || h.assetPreviewBaseURL == "" {
		return fallback
	}
	return buildAssetPreviewURL(h.assetPreviewBaseURL, key)
}

func buildAssetPreviewURL(baseURL, key string) string {
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/v1/assets/preview")
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("path", key)
	u.RawQuery = q.Encode()
	return u.String()
}

func normalizePublicAssetKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("path is required")
	}
	if strings.ContainsRune(raw, 0) {
		return "", errors.New("invalid path")
	}
	if strings.HasPrefix(raw, "/") {
		return "", errors.New("path must be relative")
	}
	if strings.Contains(raw, `\`) {
		return "", errors.New("path must use forward slashes")
	}
	for seg := range strings.SplitSeq(raw, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", errors.New("path must be normalized")
		}
	}
	if cleaned := path.Clean(raw); cleaned != raw {
		return "", errors.New("path must be normalized")
	}
	for _, prefix := range allowedPublicAssetPrefixes {
		if strings.HasPrefix(raw, prefix) {
			return raw, nil
		}
	}
	return "", errors.New("unsupported asset path")
}
