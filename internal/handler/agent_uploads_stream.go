package handler

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// authAgent resolves the agent and session sandbox from the URL params
// and verifies the bearer matches the sandbox's runtime secret. On failure
// it writes the JSON error response and returns false — callers must return.
func (h *UploadsHandler) authAgent(w http.ResponseWriter, r *http.Request) (*model.Agent, *model.Sandbox, bool) {
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "drive endpoints not configured"})
		return nil, nil, false
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return nil, nil, false
	}
	sandboxID, err := uuid.Parse(chi.URLParam(r, "sandboxID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sandbox_id"})
		return nil, nil, false
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil, nil, false
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND status <> ?", agentID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return nil, nil, false
	}
	if agent.OrgID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent has no org"})
		return nil, nil, false
	}

	var sandbox model.Sandbox
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND agent_id = ?", sandboxID, *agent.OrgID, agentID).
		First(&sandbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil, nil, false
	}

	wantKey, err := h.encKey.DecryptString(sandbox.EncryptedRuntimeSecret)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt runtime secret", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid runtime secret"})
		return nil, nil, false
	}

	return &agent, &sandbox, true
}

func buildAgentAssetKey(agentID uuid.UUID, folder, filename string) string {
	if folder == "" {
		return fmt.Sprintf("pub/e/%s/%s", agentID, filename)
	}
	return fmt.Sprintf("pub/e/%s/%s/%s", agentID, folder, filename)
}

// StreamAgentAsset accepts a streamed PUT body and stores it in the
// agent drive. Auth: bearer must equal the agent sandbox's runtime API
// key.
//
//	PUT /internal/agents/{agentID}/sandboxes/{sandboxID}/drive/*
func (h *UploadsHandler) StreamAgentAsset(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "streaming uploads not configured"})
		return
	}

	folder, filename, perr := splitAssetPath(chi.URLParam(r, "*"))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}

	agent, sandbox, ok := h.authAgent(w, r)
	if !ok {
		return
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		if guessed := mime.TypeByExtension(filepath.Ext(filename)); guessed != "" {
			contentType = guessed
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	key := buildAgentAssetKey(agent.ID, folder, filename)
	assetURL := h.publicAssetURL(key, "")

	stored, err := h.streamer.Stream(r.Context(), key, contentType, r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "agent drive stream failed", "agent_id", agent.ID, "key", key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}
	if stored.Bytes == 0 {
		if err := h.streamer.Delete(r.Context(), stored.Key); err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete empty agent drive object", "agent_id", agent.ID, "key", stored.Key, "error", err)
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "empty files are not allowed"})
		return
	}

	asset := model.AgentAsset{
		AgentID:     agent.ID,
		OrgID:       *agent.OrgID,
		SandboxID:   &sandbox.ID,
		Path:        folder,
		Filename:    filename,
		Key:         stored.Key,
		PublicURL:   assetURL,
		ContentType: contentType,
		Bytes:       stored.Bytes,
	}
	if err := h.db.Where("key = ?", stored.Key).Assign(map[string]any{
		"agent_id":            agent.ID,
		"org_id":              *agent.OrgID,
		"sandbox_id":          sandbox.ID,
		"path":                folder,
		"filename":            filename,
		assetURLStorageColumn: assetURL,
		"content_type":        contentType,
		"bytes":               stored.Bytes,
		"updated_at":          time.Now(),
	}).FirstOrCreate(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "persist agent drive file", "agent_id", agent.ID, "key", stored.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save asset"})
		return
	}

	writeJSON(w, http.StatusCreated, streamAssetResponse{
		ID:          asset.ID,
		PublicURL:   h.publicAssetURL(stored.Key, assetURL),
		Key:         stored.Key,
		Path:        folder,
		Filename:    filename,
		ContentType: contentType,
		Bytes:       stored.Bytes,
	})
}

// DeleteAgentAsset removes both the S3 object and the DB row.
//
//	DELETE /internal/agents/{agentID}/sandboxes/{sandboxID}/drive/*
func (h *UploadsHandler) DeleteAgentAsset(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "drive endpoints not configured"})
		return
	}

	folder, filename, perr := splitAssetPath(chi.URLParam(r, "*"))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}

	agent, sandbox, ok := h.authAgent(w, r)
	if !ok {
		return
	}

	key := buildAgentAssetKey(agent.ID, folder, filename)

	var asset model.AgentAsset
	if err := h.db.Where("agent_id = ? AND sandbox_id = ? AND key = ?", agent.ID, sandbox.ID, key).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load asset"})
		return
	}

	if err := h.streamer.Delete(r.Context(), asset.Key); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete s3 object", "agent_id", agent.ID, "key", asset.Key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete object"})
		return
	}
	if err := h.db.Delete(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete asset row", "agent_id", agent.ID, "key", asset.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete asset"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
