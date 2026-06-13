package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const defaultAgentSQLiteBackupMaxBytes int64 = 5 * 1024 * 1024 * 1024
const agentSQLiteBackupReadTimeout = 10 * time.Minute

type agentSQLiteBackupStreamer interface {
	Stream(ctx context.Context, key string, body io.Reader, contentType string) error
}

type AgentSQLiteBackupHandler struct {
	db                *gorm.DB
	storage           agentSQLiteBackupStreamer
	encKey            *crypto.SymmetricKey
	maxBytes          int64
	agentRuntimeImage string
}

func NewAgentSQLiteBackupHandler(db *gorm.DB, s3 agentSQLiteBackupStreamer, encKey *crypto.SymmetricKey, maxBytes int64) *AgentSQLiteBackupHandler {
	if maxBytes <= 0 {
		maxBytes = defaultAgentSQLiteBackupMaxBytes
	}
	return &AgentSQLiteBackupHandler{db: db, storage: s3, encKey: encKey, maxBytes: maxBytes}
}

func (h *AgentSQLiteBackupHandler) WithRuntimeImages(agentImage string) *AgentSQLiteBackupHandler {
	h.agentRuntimeImage = agentImage
	return h
}

func (h *AgentSQLiteBackupHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.storage == nil || h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sqlite backup endpoint not configured"})
		return
	}
	if setAgentSQLiteBackupReadDeadline(w, time.Now().Add(agentSQLiteBackupReadTimeout)) {
		defer setAgentSQLiteBackupReadDeadline(w, time.Time{})
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}
	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/gzip" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content-type must be application/gzip"})
		return
	}
	if r.ContentLength > h.maxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "backup exceeds maximum size"})
		return
	}

	agent, sandbox, ok := h.authenticateAgentRuntime(w, r, agentID, bearer)
	if !ok {
		return
	}

	upgradeID, ok := h.parseAndVerifyUpgradeID(w, r, agent)
	if !ok {
		return
	}

	key := agentSQLiteBackupKey(*agent.OrgID, agent.ID, upgradeID)
	body := http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := h.storage.Stream(r.Context(), key, body, "application/gzip"); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "backup exceeds maximum size"})
			return
		}
		attrs := []any{
			"agent_id", agent.ID,
			"sandbox_id", sandbox.ID,
			"key", key,
			"error", err,
		}
		if upgradeID != nil {
			attrs = append(attrs, "upgrade_id", *upgradeID)
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "agent sqlite backup upload failed", attrs...)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "key": key})
}

func setAgentSQLiteBackupReadDeadline(w http.ResponseWriter, deadline time.Time) bool {
	return http.NewResponseController(w).SetReadDeadline(deadline) == nil
}

func agentSQLiteBackupKey(orgID, agentID uuid.UUID, upgradeID *uuid.UUID) string {
	if upgradeID != nil {
		return fmt.Sprintf("agent-sqlite-backups/%s/%s/upgrades/%s.db.gz", orgID, agentID, *upgradeID)
	}
	return fmt.Sprintf("agent-sqlite-backups/%s/%s/latest.db.gz", orgID, agentID)
}

func (h *AgentSQLiteBackupHandler) parseAndVerifyUpgradeID(w http.ResponseWriter, r *http.Request, agent *model.Agent) (*uuid.UUID, bool) {
	raw := r.URL.Query().Get("upgrade_id")
	if raw == "" {
		return nil, true
	}
	upgradeID, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upgrade_id"})
		return nil, false
	}
	var count int64
	err = h.db.WithContext(r.Context()).Model(&model.AgentSandboxUpgrade{}).
		Where("id = ? AND org_id = ? AND agent_id = ?", upgradeID, *agent.OrgID, agent.ID).
		Count(&count).Error
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify upgrade"})
		return nil, false
	}
	if count == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upgrade not found"})
		return nil, false
	}
	return &upgradeID, true
}

func (h *AgentSQLiteBackupHandler) authenticateAgentRuntime(w http.ResponseWriter, r *http.Request, agentID uuid.UUID, bearer string) (*model.Agent, *model.Sandbox, bool) {
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

	sandbox, err := h.agentRuntimeSelector().MainRuntime(r.Context(), *agent.OrgID, agentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found for agent"})
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
	return &agent, sandbox, true
}

func (h *AgentSQLiteBackupHandler) agentRuntimeSelector() agentsandbox.Selector {
	return agentsandbox.Selector{
		DB:                h.db,
		AgentRuntimeImage: h.agentRuntimeImage,
	}
}
