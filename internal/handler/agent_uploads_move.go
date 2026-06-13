package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// MoveAgentAsset relabels a drive file's folder. Only the database `path`
// column is touched — the S3 key (and therefore the asset URL) stays put.
//
//	POST /internal/agents/{agentID}/drive/move
//	body: {"asset":"<asset_url|folder/filename>","new_path":"archive"}
func (h *UploadsHandler) MoveAgentAsset(w http.ResponseWriter, r *http.Request) {
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "drive endpoints not configured"})
		return
	}

	agent, _, ok := h.authAgent(w, r)
	if !ok {
		return
	}

	var req moveAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	folder, filename, err := resolveAgentAssetReference(agent.ID, req.Asset)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	newFolder, err := validateFolder(req.NewPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	key := buildAgentAssetKey(agent.ID, folder, filename)
	var asset model.AgentAsset
	if err := h.db.Where("agent_id = ? AND key = ?", agent.ID, key).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load asset"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&asset).Updates(map[string]any{
		"path":       newFolder,
		"updated_at": now,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update asset"})
		return
	}

	writeJSON(w, http.StatusOK, moveAssetResponse{
		ID:        asset.ID,
		PublicURL: h.publicAssetURL(asset.Key, asset.PublicURL),
		Key:       asset.Key,
		Path:      newFolder,
		Filename:  asset.Filename,
		UpdatedAt: now.UTC().Format(time.RFC3339),
	})
}

func resolveAgentAssetReference(agentID uuid.UUID, raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("asset is required")
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", errors.New("invalid asset URL")
		}
		if u.Path == "/v1/assets/preview" {
			key, err := normalizePublicAssetKey(u.Query().Get("path"))
			if err != nil {
				return "", "", errors.New("invalid asset URL")
			}
			marker := fmt.Sprintf("pub/e/%s/", agentID)
			if !strings.HasPrefix(key, marker) {
				return "", "", errors.New("asset URL does not belong to this agent")
			}
			raw = strings.TrimPrefix(key, marker)
			return splitAssetPath(raw)
		}
		marker := fmt.Sprintf("/pub/e/%s/", agentID)
		idx := strings.Index(u.Path, marker)
		if idx < 0 {
			return "", "", errors.New("asset URL does not belong to this agent")
		}
		raw = u.Path[idx+len(marker):]
	}

	return splitAssetPath(raw)
}
